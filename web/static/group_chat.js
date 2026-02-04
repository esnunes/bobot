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
        this.leaveBtn = document.getElementById('leave-btn');  // May be null if owner
        this.deleteBtn = document.getElementById('delete-btn');  // May be null if not owner
        this.mentionBotBtn = document.getElementById('mention-bot-btn');
        this.isLoadingHistory = false;
        this.oldestMessageId = null;
        this.hasMoreHistory = true;
        this.currentUserId = null;
        this.wsContainer = document.getElementById('ws-connection');
        this.handleGroupMessage = null;

        this.init();
    }

    init() {
        this.wsContainer.connect();
        this.initFromDOM();
        this.setupEventListeners();
        this.scrollToBottom();
    }

    initFromDOM() {
        const container = document.querySelector('[data-page="group-chat"]');
        this.currentUserId = parseInt(container.dataset.currentUserId, 10);

        const messageEls = this.messagesEl.querySelectorAll('[data-message-id]');
        if (messageEls.length > 0) {
            this.oldestMessageId = parseInt(messageEls[0].dataset.messageId, 10);
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

        this.form.addEventListener('htmx:confirm', (e) => {
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

        if (this.leaveBtn) {
            this.leaveBtn.addEventListener('click', () => this.leaveGroup());
        }
        if (this.deleteBtn) {
            this.deleteBtn.addEventListener('click', () => this.deleteGroup());
        }

        if (this.mentionBotBtn) {
            this.mentionBotBtn.addEventListener('click', () => this.mentionBot());
        }

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

    mentionBot() {
        const currentValue = this.input.value;
        const mention = '@bobot ';

        // Add mention at cursor position or append if no selection
        if (this.input.selectionStart !== undefined) {
            const start = this.input.selectionStart;
            this.input.value = currentValue.slice(0, start) + mention + currentValue.slice(start);
            this.input.selectionStart = this.input.selectionEnd = start + mention.length;
        } else {
            this.input.value = currentValue + mention;
        }

        this.input.focus();
    }

    sendMessage() {
        const content = this.input.value.trim();
        if (!content) return;

        if (this.wsContainer.send({ content: content, group_id: this.groupId })) {
            this.input.value = '';
            if (content.toLowerCase().includes('@bobot')) {
                this.showTypingIndicator();
            }
        }
    }

    addMessage(msg, scroll = true) {
        const msgEl = document.createElement('div');
        const role = msg.role || msg.Role;
        const content = msg.content || msg.Content;
        const displayName = msg.display_name || msg.DisplayName;

        // Map command to user styling, system to assistant styling
        const displayRole = role === 'command' ? 'user' : (role === 'system' ? 'assistant' : role);
        msgEl.className = `message ${displayRole}`;

        if ((role === 'user' || role === 'command') && displayName) {
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

        try {
            const resp = await fetch(
                `/api/groups/${this.groupId}/messages/history?before=${this.oldestMessageId}&limit=50`,
                { credentials: 'include' }
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
        // Map command to user styling, system to assistant styling
        const displayRole = role === 'command' ? 'user' : (role === 'system' ? 'assistant' : role);
        msgEl.className = `message ${displayRole}`;

        if ((role === 'user' || role === 'command') && msg.DisplayName) {
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

        try {
            const resp = await fetch(`/api/groups/${this.groupId}/members/${this.currentUserId}`, {
                method: 'DELETE',
                credentials: 'include'
            });

            if (!resp.ok) throw new Error('Failed to leave group');

            htmx.ajax('GET', '/groups', {target: 'body', swap: 'innerHTML'});
        } catch (err) {
            console.error('Failed to leave group:', err);
            alert('Failed to leave group');
        }
    }

    async deleteGroup() {
        if (!confirm('Are you sure you want to delete this group? This cannot be undone.')) return;

        try {
            const resp = await fetch(`/api/groups/${this.groupId}`, {
                method: 'DELETE',
                credentials: 'include'
            });

            if (!resp.ok) throw new Error('Failed to delete group');

            htmx.ajax('GET', '/groups', {target: 'body', swap: 'innerHTML'});
        } catch (err) {
            console.error('Failed to delete group:', err);
            alert('Failed to delete group');
        }
    }
};

var container = document.querySelector('[data-page="group-chat"]');
if (container) {
    var groupId = parseInt(container.dataset.groupId, 10);
    window.groupChatClient = new GroupChatClient(groupId);
}
