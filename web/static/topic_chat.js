window.TopicChatClient = class TopicChatClient {
    constructor(topicId) {
        // Clean up any previous page client
        if (window.currentPageClient && window.currentPageClient.cleanup) {
            window.currentPageClient.cleanup();
        }
        window.currentPageClient = this;

        this.topicId = topicId;
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
        this.handleTopicMessage = null;
        this.handleUnreadChanged = null;

        this.init();
    }

    init() {
        this.wsContainer.connect();
        this.loadInitialMessages();
        this.setupEventListeners();
        this.scrollToBottom();
    }

    loadInitialMessages() {
        var dataEl = document.querySelector('script[data-page-data]');
        if (!dataEl) return;

        var data = JSON.parse(dataEl.textContent);
        this.currentUserId = data.current_user_id;

        var messages = data.messages || [];
        messages.forEach(function(msg) {
            this.addMessage(msg, false);
        }.bind(this));

        if (messages.length > 0) {
            this.oldestMessageId = messages[0].id;
        }
    }

    setupEventListeners() {
        // WebSocket message events - filter by topic_id
        this.handleTopicMessage = (event) => {
            const data = event.detail;
            if (data.topic_id === this.topicId) {
                if (data.role === 'assistant') {
                    this.removeTypingIndicator();
                }
                this.addMessage(data, true);
                // Show typing indicator after user message with @bobot is displayed
                if (data.role === 'user' && data.content && data.content.toLowerCase().includes('@bobot')) {
                    this.showTypingIndicator();
                }
            }
        };
        document.addEventListener('bobot:topic-message', this.handleTopicMessage);

        this.form.addEventListener('submit', (e) => {
            e.preventDefault();
            this.sendMessage();
            return false;
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
            this.leaveBtn.addEventListener('click', () => this.leaveTopic());
        }
        if (this.deleteBtn) {
            this.deleteBtn.addEventListener('click', () => this.deleteTopic());
        }

        if (this.mentionBotBtn) {
            this.mentionBotBtn.addEventListener('click', () => this.mentionBot());
        }

        // Unread indicator on back button
        this.handleUnreadChanged = (e) => {
            var btn = document.querySelector('button[aria-label="Chats"]');
            if (!btn) return;
            var dot = btn.querySelector('.unread-dot');
            if (e.detail.chatIds.size > 0) {
                if (!dot) {
                    dot = document.createElement('span');
                    dot.className = 'unread-dot';
                    btn.appendChild(dot);
                }
            } else {
                if (dot) dot.remove();
            }
        };
        document.addEventListener('bobot:unread-changed', this.handleUnreadChanged);

        // Reset bobot confirm buttons when clicking elsewhere
        document.addEventListener('click', () => {
            MessageRenderer.resetConfirmingButtons();
        });

        this.messagesEl.addEventListener('scroll', () => {
            if (this.messagesEl.scrollTop < 100) {
                this.loadMoreHistory();
            }
        });
    }

    cleanup() {
        if (this.handleTopicMessage) {
            document.removeEventListener('bobot:topic-message', this.handleTopicMessage);
            this.handleTopicMessage = null;
        }
        if (this.handleUnreadChanged) {
            document.removeEventListener('bobot:unread-changed', this.handleUnreadChanged);
            this.handleUnreadChanged = null;
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

        if (this.wsContainer.send({ content: content, topic_id: this.topicId })) {
            this.input.value = '';
        }
    }

    addMessage(msg, scroll = true) {
        const msgEl = document.createElement('div');
        const role = msg.role || msg.Role;
        const content = msg.content || msg.Content;
        const displayName = msg.display_name || msg.DisplayName;
        const userId = msg.user_id || msg.UserID;
        const id = msg.id || msg.ID;
        const self = (userId === this.currentUserId) ? ' self' : '';

        msgEl.className = `message ${role}${self}`;

        if (displayName) {
            const nameEl = document.createElement('div');
            nameEl.className = 'message-sender';
            nameEl.textContent = displayName;
            msgEl.appendChild(nameEl);
        }

        const contentEl = document.createElement('div');
        contentEl.className = 'message-content';

        var html = MessageRenderer.renderMessageContent(content, role);
        if (html !== null) {
            contentEl.innerHTML = html;
            contentEl.classList.add('markdown-content');
            // Highlight after inserting into DOM
            msgEl.appendChild(contentEl);
            MessageRenderer.highlightCodeBlocks(contentEl);
            MessageRenderer.processBobotTags(contentEl, (msg) => {
                this.wsContainer.send({ content: msg, topic_id: this.topicId });
            }, !!id);
        } else {
            var scheduled = MessageRenderer.parseScheduledMessage(content);
            if (scheduled) {
                contentEl.appendChild(MessageRenderer.renderScheduledMessage(scheduled));
            } else {
                contentEl.textContent = content;
            }
            msgEl.appendChild(contentEl);
        }

        if (id) {
            msgEl.setAttribute('data-message-id', id);
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
                `/api/topics/${this.topicId}/messages/history?before=${this.oldestMessageId}&limit=50`,
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
        const role = msg.role || msg.Role;
        const content = msg.content || msg.Content;
        const displayName = msg.display_name || msg.DisplayName;
        const userId = msg.user_id || msg.UserID;
        const id = msg.id || msg.ID;
        const self = (userId === this.currentUserId) ? ' self' : '';
        msgEl.className = `message ${role}${self}`;

        if (displayName) {
            const nameEl = document.createElement('div');
            nameEl.className = 'message-sender';
            nameEl.textContent = displayName;
            msgEl.appendChild(nameEl);
        }

        const contentEl = document.createElement('div');
        contentEl.className = 'message-content';

        var html = MessageRenderer.renderMessageContent(content, role);
        if (html !== null) {
            contentEl.innerHTML = html;
            contentEl.classList.add('markdown-content');
            msgEl.appendChild(contentEl);
            MessageRenderer.highlightCodeBlocks(contentEl);
            MessageRenderer.processBobotTags(contentEl, (msg) => {
                this.wsContainer.send({ content: msg, topic_id: this.topicId });
            }, true);
        } else {
            var scheduled = MessageRenderer.parseScheduledMessage(content);
            if (scheduled) {
                contentEl.appendChild(MessageRenderer.renderScheduledMessage(scheduled));
            } else {
                contentEl.textContent = content;
            }
            msgEl.appendChild(contentEl);
        }

        if (id) {
            msgEl.setAttribute('data-message-id', id);
        }

        this.messagesEl.insertBefore(msgEl, this.messagesEl.firstChild);
    }

    async leaveTopic() {
        if (!confirm('Are you sure you want to leave this topic?')) return;

        try {
            const resp = await fetch(`/api/topics/${this.topicId}/members/${this.currentUserId}`, {
                method: 'DELETE',
                credentials: 'include'
            });

            if (!resp.ok) throw new Error('Failed to leave topic');

            htmx.ajax('GET', '/chats', {target: 'body', swap: 'innerHTML'});
        } catch (err) {
            console.error('Failed to leave topic:', err);
            alert('Failed to leave topic');
        }
    }

    async deleteTopic() {
        if (!confirm('Are you sure you want to delete this topic? This cannot be undone.')) return;

        try {
            const resp = await fetch(`/api/topics/${this.topicId}`, {
                method: 'DELETE',
                credentials: 'include'
            });

            if (!resp.ok) throw new Error('Failed to delete topic');

            htmx.ajax('GET', '/chats', {target: 'body', swap: 'innerHTML'});
        } catch (err) {
            console.error('Failed to delete topic:', err);
            alert('Failed to delete topic');
        }
    }
};

var container = document.querySelector('[data-page="topic-chat"]');
if (container) {
    var topicId = parseInt(container.dataset.topicId, 10);
    window.topicChatClient = new TopicChatClient(topicId);
}
