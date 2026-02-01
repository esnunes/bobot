// Guard against re-declaration on HTMX swaps
if (typeof GroupChatClient === 'undefined') {
    window.GroupChatClient = class GroupChatClient {
        constructor(groupId) {
            // Clean up any previous page client
            if (window.currentPageClient && window.currentPageClient.cleanup) {
                window.currentPageClient.cleanup();
            }
            window.currentPageClient = this;

            this.groupId = groupId;
            this.messagesEl = document.getElementById('messages');
            this.form = document.getElementById('chat-form');
            this.input = document.getElementById('message-input');
            this.menuBtn = document.getElementById('menu-btn');
            this.menuOverlay = document.getElementById('menu-overlay');
            this.leaveBtn = document.getElementById('leave-btn');
            this.deleteBtn = document.getElementById('delete-btn');
            this.isLoadingHistory = false;
            this.oldestMessageId = null;
            this.hasMoreHistory = true;
            this.currentUserId = null;
            this.wsContainer = document.getElementById('ws-connection');
            this.handleGroupMessage = null;

            this.init();
        }

        async init() {
            const token = localStorage.getItem('access_token');
            if (!token) {
                htmx.ajax('GET', '/', {target: 'body', swap: 'innerHTML'}).then(() => {
                    history.pushState({}, '', '/');
                });
                return;
            }

            await this.loadGroupInfo(token);
            await this.loadRecentMessages(token);
            this.setupEventListeners();
        }

        async loadGroupInfo(token) {
            try {
                const resp = await fetch(`/api/groups/${this.groupId}`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

                if (!resp.ok) {
                    if (resp.status === 403 || resp.status === 404) {
                        htmx.ajax('GET', '/groups', {target: 'body', swap: 'innerHTML'}).then(() => {
                            history.pushState({}, '', '/groups');
                        });
                        return;
                    }
                    throw new Error('Failed to load group');
                }

                const group = await resp.json();
                document.getElementById('group-name').textContent = group.name;

                const payload = JSON.parse(atob(token.split('.')[1]));
                this.currentUserId = payload.user_id;

                if (group.owner_id === this.currentUserId) {
                    this.deleteBtn.classList.remove('hidden');
                    this.leaveBtn.classList.add('hidden');
                }

                const membersList = document.getElementById('members-list');
                membersList.innerHTML = '<strong>Members:</strong>' + group.members.map(m =>
                    `<div class="member">${this.escapeHtml(m.display_name || m.username)}</div>`
                ).join('');

            } catch (err) {
                console.error('Failed to load group:', err);
            }
        }

        async loadRecentMessages(token) {
            try {
                const resp = await fetch(`/api/groups/${this.groupId}/messages/recent?limit=50`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

                if (!resp.ok) throw new Error('Failed to load messages');

                const messages = await resp.json();
                if (messages && messages.length > 0) {
                    messages.forEach(msg => this.addMessage(msg, false));
                    this.oldestMessageId = messages[0].ID;
                }
                this.scrollToBottom();
            } catch (err) {
                console.error('Failed to load messages:', err);
            }
        }

        setupEventListeners() {
            // WebSocket message events - filter by group_id
            this.handleGroupMessage = (event) => {
                const data = event.detail;
                if (data.group_id === this.groupId) {
                    if (data.role === 'assistant') {
                        this.removeTypingIndicator();
                    }
                    this.addMessage(data, true);
                }
            };
            document.addEventListener('bobot:group-message', this.handleGroupMessage);

            this.form.addEventListener('submit', (e) => {
                e.preventDefault();
                this.sendMessage();
            });

            this.menuBtn.addEventListener('click', () => {
                this.menuOverlay.classList.remove('hidden');
            });

            this.menuOverlay.addEventListener('click', (e) => {
                if (e.target === this.menuOverlay) {
                    this.menuOverlay.classList.add('hidden');
                }
            });

            this.leaveBtn.addEventListener('click', () => this.leaveGroup());
            this.deleteBtn.addEventListener('click', () => this.deleteGroup());

            this.messagesEl.addEventListener('scroll', () => {
                if (this.messagesEl.scrollTop < 100) {
                    this.loadMoreHistory();
                }
            });
        }

        cleanup() {
            if (this.handleGroupMessage) {
                document.removeEventListener('bobot:group-message', this.handleGroupMessage);
                this.handleGroupMessage = null;
            }
        }

        sendMessage() {
            const content = this.input.value.trim();
            if (!content) return;

            if (this.wsContainer.send({ content: content, group_id: this.groupId })) {
                this.input.value = '';
                if (content.toLowerCase().includes('@assistant')) {
                    this.showTypingIndicator();
                }
            }
        }

        addMessage(msg, scroll = true) {
            const msgEl = document.createElement('div');
            const role = msg.role || msg.Role;
            const content = msg.content || msg.Content;
            const displayName = msg.display_name || msg.DisplayName;

            msgEl.className = `message ${role}`;

            if (role === 'user' && displayName) {
                const nameEl = document.createElement('div');
                nameEl.className = 'message-sender';
                nameEl.textContent = displayName;
                msgEl.appendChild(nameEl);
            }

            const contentEl = document.createElement('div');
            contentEl.className = 'message-content';
            contentEl.textContent = content;
            msgEl.appendChild(contentEl);

            if (msg.ID) {
                msgEl.setAttribute('data-message-id', msg.ID);
            }

            this.messagesEl.appendChild(msgEl);
            if (scroll) this.scrollToBottom();
        }

        showTypingIndicator() {
            const indicator = document.createElement('div');
            indicator.className = 'message assistant typing-indicator';
            indicator.id = 'typing-indicator';
            indicator.innerHTML = '<span></span><span></span><span></span>';
            this.messagesEl.appendChild(indicator);
            this.scrollToBottom();
        }

        removeTypingIndicator() {
            const indicator = document.getElementById('typing-indicator');
            if (indicator) indicator.remove();
        }

        scrollToBottom() {
            this.messagesEl.scrollTop = this.messagesEl.scrollHeight;
        }

        async loadMoreHistory() {
            if (this.isLoadingHistory || !this.hasMoreHistory || !this.oldestMessageId) return;

            this.isLoadingHistory = true;
            const token = localStorage.getItem('access_token');

            try {
                const resp = await fetch(
                    `/api/groups/${this.groupId}/messages/history?before=${this.oldestMessageId}&limit=50`,
                    { headers: { 'Authorization': `Bearer ${token}` } }
                );

                if (!resp.ok) throw new Error('Failed to load history');

                const messages = await resp.json();
                if (messages && messages.length > 0) {
                    const scrollHeight = this.messagesEl.scrollHeight;
                    const scrollTop = this.messagesEl.scrollTop;

                    messages.reverse().forEach(msg => this.prependMessage(msg));
                    this.oldestMessageId = messages[0].ID;

                    this.messagesEl.scrollTop = this.messagesEl.scrollHeight - scrollHeight + scrollTop;
                } else {
                    this.hasMoreHistory = false;
                }
            } catch (err) {
                console.error('Failed to load history:', err);
            } finally {
                this.isLoadingHistory = false;
            }
        }

        prependMessage(msg) {
            const msgEl = document.createElement('div');
            const role = msg.Role;
            msgEl.className = `message ${role}`;

            if (role === 'user' && msg.DisplayName) {
                const nameEl = document.createElement('div');
                nameEl.className = 'message-sender';
                nameEl.textContent = msg.DisplayName;
                msgEl.appendChild(nameEl);
            }

            const contentEl = document.createElement('div');
            contentEl.className = 'message-content';
            contentEl.textContent = msg.Content;
            msgEl.appendChild(contentEl);

            if (msg.ID) {
                msgEl.setAttribute('data-message-id', msg.ID);
            }

            this.messagesEl.insertBefore(msgEl, this.messagesEl.firstChild);
        }

        async leaveGroup() {
            if (!confirm('Are you sure you want to leave this group?')) return;

            const token = localStorage.getItem('access_token');
            try {
                const resp = await fetch(`/api/groups/${this.groupId}/members/${this.currentUserId}`, {
                    method: 'DELETE',
                    headers: { 'Authorization': `Bearer ${token}` }
                });

                if (!resp.ok) throw new Error('Failed to leave group');

                htmx.ajax('GET', '/groups', {target: 'body', swap: 'innerHTML'}).then(() => {
                    history.pushState({}, '', '/groups');
                });
            } catch (err) {
                console.error('Failed to leave group:', err);
                alert('Failed to leave group');
            }
        }

        async deleteGroup() {
            if (!confirm('Are you sure you want to delete this group? This cannot be undone.')) return;

            const token = localStorage.getItem('access_token');
            try {
                const resp = await fetch(`/api/groups/${this.groupId}`, {
                    method: 'DELETE',
                    headers: { 'Authorization': `Bearer ${token}` }
                });

                if (!resp.ok) throw new Error('Failed to delete group');

                htmx.ajax('GET', '/groups', {target: 'body', swap: 'innerHTML'}).then(() => {
                    history.pushState({}, '', '/groups');
                });
            } catch (err) {
                console.error('Failed to delete group:', err);
                alert('Failed to delete group');
            }
        }

        escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
    };
}

// Auto-initialize when group chat page is loaded
(function() {
    var container = document.querySelector('[data-page="group-chat"]');
    if (container) {
        var groupId = parseInt(container.dataset.groupId, 10);
        window.groupChatClient = new GroupChatClient(groupId);
    }
})();
