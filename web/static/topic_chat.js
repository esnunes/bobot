var QUICK_ACTIONS = [
    { label: 'Turn on AC', message: '@bobot turn on the AC in the living room', mode: 'send' },
    { label: 'Turn off lights', message: '@bobot turn off all the lights in the house', mode: 'send' },
    { label: 'Check weather', message: "@bobot what's the weather like today?", mode: 'send' },
    { label: 'Set a reminder', message: '@bobot remind me to ', mode: 'fill' },
    { label: 'Morning routine', message: '@bobot start my morning routine', mode: 'send' },
    { label: 'Grocery list', message: '@bobot add to my grocery list: ', mode: 'fill' },
];

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
        this.mentionBotBtn = document.getElementById('mention-bot-btn');
        this.quickActionsBtn = document.getElementById('quick-actions-btn');
        this.quickActionsOverlay = document.getElementById('quick-actions-overlay');
        this.quickActionsClose = document.getElementById('quick-actions-close');
        this.quickActionsList = document.getElementById('quick-actions-list');
        this.isLoadingHistory = false;
        this.oldestMessageId = null;
        this.hasMoreHistory = true;
        this.currentUserId = null;
        this.autoRespond = false;
        this.wsContainer = document.getElementById('ws-connection');
        this.handleTopicMessage = null;
        this.handleUnreadChanged = null;

        this.init();
    }

    init() {
        this.wsContainer.connect();
        this.loadInitialMessages();
        this.setupEventListeners();
        this.setupQuickActions();
        this.scrollToBottom();
    }

    loadInitialMessages() {
        var dataEl = document.querySelector('script[data-page-data]');
        if (!dataEl) return;

        var data = JSON.parse(dataEl.textContent);
        this.currentUserId = data.current_user_id;
        this.autoRespond = !!data.auto_respond;

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
                // Show typing indicator when assistant will respond
                if (data.role === 'user') {
                    if (this.autoRespond || (data.content && data.content.toLowerCase().includes('@bobot'))) {
                        this.showTypingIndicator();
                    }
                }
            }
        };
        document.addEventListener('bobot:topic-message', this.handleTopicMessage);

        this.form.addEventListener('submit', (e) => {
            e.preventDefault();
            this.sendMessage();
            return false;
        });

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
        if (this.handleQuickActionsKeydown) {
            document.removeEventListener('keydown', this.handleQuickActionsKeydown);
            this.handleQuickActionsKeydown = null;
        }
    }

    mentionBot() {
        const mention = '@bobot ';

        if (!this.input.value.toLowerCase().startsWith(mention.trimEnd().toLowerCase())) {
            this.input.value = mention + this.input.value;
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

    setupQuickActions() {
        if (!this.quickActionsBtn || !this.quickActionsOverlay) return;

        // Render action items
        QUICK_ACTIONS.forEach(function(action) {
            var btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'quick-action-item';
            btn.setAttribute('role', 'button');

            var labelEl = document.createElement('span');
            labelEl.className = 'quick-action-label';
            labelEl.textContent = action.label;

            if (action.mode === 'fill') {
                var icon = document.createElement('svg');
                icon.setAttribute('width', '14');
                icon.setAttribute('height', '14');
                icon.setAttribute('viewBox', '0 0 24 24');
                icon.setAttribute('fill', 'none');
                icon.setAttribute('stroke', 'currentColor');
                icon.setAttribute('stroke-width', '2');
                icon.setAttribute('stroke-linecap', 'round');
                icon.setAttribute('stroke-linejoin', 'round');
                icon.className = 'quick-action-mode-icon';
                icon.innerHTML = '<path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/>';
                labelEl.appendChild(icon);
            }

            var previewEl = document.createElement('span');
            previewEl.className = 'quick-action-preview';
            previewEl.textContent = action.message;

            btn.appendChild(labelEl);
            btn.appendChild(previewEl);

            btn.addEventListener('click', function() {
                this.handleQuickAction(action);
            }.bind(this));

            this.quickActionsList.appendChild(btn);
        }.bind(this));

        // Open overlay
        this.quickActionsBtn.addEventListener('click', function() {
            this.openQuickActions();
        }.bind(this));

        // Close via X button
        this.quickActionsClose.addEventListener('click', function() {
            this.closeQuickActions();
        }.bind(this));

        // Escape key
        this.handleQuickActionsKeydown = function(e) {
            if (e.key === 'Escape' && !this.quickActionsOverlay.classList.contains('hidden')) {
                this.closeQuickActions();
            }
        }.bind(this);
        document.addEventListener('keydown', this.handleQuickActionsKeydown);
    }

    openQuickActions() {
        this.quickActionsOverlay.classList.remove('hidden');
        this.quickActionsClose.focus();
    }

    closeQuickActions() {
        this.quickActionsOverlay.classList.add('hidden');
        this.quickActionsBtn.focus();
    }

    handleQuickAction(action) {
        if (action.mode === 'fill') {
            this.input.value = action.message;
            this.closeQuickActions();
            this.input.focus();
        } else {
            this.wsContainer.send({ content: action.message, topic_id: this.topicId });
            this.closeQuickActions();
        }
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

};

var container = document.querySelector('[data-page="topic-chat"]');
if (container) {
    var topicId = parseInt(container.dataset.topicId, 10);
    window.topicChatClient = new TopicChatClient(topicId);
}
