class GroupChatClient {
    constructor(groupId) {
        this.groupId = groupId;
        this.ws = null;
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

        this.init();
    }

    async init() {
        const token = localStorage.getItem('access_token');
        if (!token) {
            window.location.href = '/';
            return;
        }

        await this.loadGroupInfo(token);
        await this.loadRecentMessages(token);
        this.connect(token);
        this.setupEventListeners();
    }

    async loadGroupInfo(token) {
        try {
            const resp = await fetch(`/api/groups/${this.groupId}`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) {
                if (resp.status === 403 || resp.status === 404) {
                    window.location.href = '/groups';
                    return;
                }
                throw new Error('Failed to load group');
            }

            const group = await resp.json();
            document.getElementById('group-name').textContent = group.name;

            // Parse JWT to get current user ID
            const payload = JSON.parse(atob(token.split('.')[1]));
            this.currentUserId = payload.user_id;

            // Show delete button if owner
            if (group.owner_id === this.currentUserId) {
                this.deleteBtn.classList.remove('hidden');
                this.leaveBtn.classList.add('hidden');
            }

            // Render members
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

    connect(token) {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/chat?token=${token}`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => console.log('WebSocket connected');

        this.ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            // Only handle messages for this group
            if (data.group_id === this.groupId) {
                if (data.role === 'assistant') {
                    this.removeTypingIndicator();
                }
                this.addMessage(data, true);
            }
        };

        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            setTimeout(() => this.reconnect(), 1000);
        };
    }

    async reconnect() {
        const refreshToken = localStorage.getItem('refresh_token');
        if (!refreshToken) {
            window.location.href = '/';
            return;
        }

        try {
            const resp = await fetch('/api/refresh', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ refresh_token: refreshToken })
            });

            if (!resp.ok) throw new Error('Refresh failed');

            const data = await resp.json();
            localStorage.setItem('access_token', data.access_token);
            this.connect(data.access_token);
        } catch (err) {
            console.error('Reconnect failed:', err);
            window.location.href = '/';
        }
    }

    setupEventListeners() {
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

    sendMessage() {
        const content = this.input.value.trim();
        if (!content || !this.ws || this.ws.readyState !== WebSocket.OPEN) return;

        this.ws.send(JSON.stringify({
            content: content,
            group_id: this.groupId
        }));
        this.input.value = '';

        if (content.toLowerCase().includes('@assistant')) {
            this.showTypingIndicator();
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
            window.location.href = '/groups';
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
            window.location.href = '/groups';
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
}

document.addEventListener('DOMContentLoaded', () => new GroupChatClient(GROUP_ID));
