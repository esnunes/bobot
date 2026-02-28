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
        this.quickActions = data.quick_actions || [];
        this.canManageQuickActions = !!data.can_manage_quick_actions;

        // Build member map for color assignment and Gravatar URLs
        this.memberMap = {};
        var members = data.members || [];
        members.forEach(function(m, index) {
            this.memberMap[m.user_id] = {
                displayName: m.display_name,
                gravatarURL: m.gravatar_url,
                colorIndex: index % 6,
            };
        }.bind(this));


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
        if (this.handleBeforeSwap) {
            document.removeEventListener('htmx:beforeSwap', this.handleBeforeSwap);
            this.handleBeforeSwap = null;
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

    _getMsgUserId(msg) {
        var role = msg.role || msg.Role;
        var userId = msg.user_id || msg.UserID;
        // Assistant and system messages are from bobot (user_id=0)
        if (role === 'assistant' || role === 'system') return 0;
        return userId || 0;
    }

    _getMemberInfo(userId) {
        if (this.memberMap && this.memberMap[userId]) {
            return this.memberMap[userId];
        }
        // Fallback for unknown/departed members
        return { displayName: '', gravatarURL: 'https://www.gravatar.com/avatar/?d=mp&s=80', colorIndex: -1 };
    }

    _buildMessageEl(msg) {
        const msgEl = document.createElement('div');
        const role = msg.role || msg.Role;
        const content = msg.content || msg.Content;
        const displayName = msg.display_name || msg.DisplayName;
        const userId = this._getMsgUserId(msg);
        const id = msg.id || msg.ID;
        const isSelf = (userId === this.currentUserId);

        msgEl.className = `message ${role}${isSelf ? ' self' : ''}`;
        msgEl.setAttribute('data-user-id', userId);
        msgEl.setAttribute('data-role', role);

        // For non-self messages, add color and avatar outside the bubble
        if (!isSelf) {
            var member = this._getMemberInfo(userId);
            var colorVar = member.colorIndex >= 0
                ? `var(--colors-member-${member.colorIndex})`
                : 'var(--colors-text-secondary)';
            msgEl.style.setProperty('--member-color', colorVar);

            // Avatar (outside the bubble, left-aligned)
            const avatarEl = document.createElement('img');
            avatarEl.className = 'message-avatar';
            avatarEl.src = member.gravatarURL;
            avatarEl.loading = 'lazy';
            avatarEl.alt = '';
            msgEl.appendChild(avatarEl);

            // Inner wrapper for name + bubble
            const innerEl = document.createElement('div');
            innerEl.className = 'message-inner';

            if (displayName) {
                const nameEl = document.createElement('div');
                nameEl.className = 'message-sender';
                nameEl.textContent = displayName;
                innerEl.appendChild(nameEl);
            }

            const contentEl = this._renderContent(content, role, id);
            innerEl.appendChild(contentEl);
            msgEl.appendChild(innerEl);
        } else {
            // Self messages: simple layout (no avatar, no colored name)
            const contentEl = this._renderContent(content, role, id);
            msgEl.appendChild(contentEl);
        }

        if (id) {
            msgEl.setAttribute('data-message-id', id);
        }

        // Default streak class (will be updated by grouping)
        msgEl.classList.add('streak-only');

        return msgEl;
    }

    _renderContent(content, role, id) {
        const contentEl = document.createElement('div');
        contentEl.className = 'message-content';

        var html = MessageRenderer.renderMessageContent(content, role);
        if (html !== null) {
            contentEl.innerHTML = html;
            contentEl.classList.add('markdown-content');
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
        }
        return contentEl;
    }

    _isSameStreak(el1, el2) {
        if (!el1 || !el2) return false;
        // Typing indicator is not a real message
        if (el1.id === 'typing-indicator' || el2.id === 'typing-indicator') return false;
        return el1.getAttribute('data-user-id') === el2.getAttribute('data-user-id') &&
               el1.getAttribute('data-role') === el2.getAttribute('data-role');
    }

    _updateStreakClasses(msgEl) {
        if (msgEl.classList.contains('self')) return;

        var prev = msgEl.previousElementSibling;
        var next = msgEl.nextElementSibling;
        var samePrev = this._isSameStreak(prev, msgEl);
        var sameNext = this._isSameStreak(msgEl, next);

        msgEl.classList.remove('streak-only', 'streak-first', 'streak-middle', 'streak-last');
        if (samePrev && sameNext) {
            msgEl.classList.add('streak-middle');
        } else if (samePrev) {
            msgEl.classList.add('streak-last');
        } else if (sameNext) {
            msgEl.classList.add('streak-first');
        } else {
            msgEl.classList.add('streak-only');
        }
    }

    _updateGroupingOnAppend(newEl) {
        // Update the previous sibling's streak class (it may have changed)
        var prev = newEl.previousElementSibling;
        if (prev && prev.id !== 'typing-indicator') {
            this._updateStreakClasses(prev);
        }
        this._updateStreakClasses(newEl);
    }

    _updateGroupingOnPrepend(newEl) {
        // Update the next sibling's streak class (it may have changed)
        var next = newEl.nextElementSibling;
        if (next && next.id !== 'typing-indicator') {
            this._updateStreakClasses(next);
        }
        this._updateStreakClasses(newEl);
    }

    addMessage(msg, scroll = true) {
        const msgEl = this._buildMessageEl(msg);
        this.messagesEl.appendChild(msgEl);
        this._updateGroupingOnAppend(msgEl);
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

        var i18nEl = document.querySelector('script[data-i18n]');
        var i18n = i18nEl ? JSON.parse(i18nEl.textContent) : {};

        // Render action items or empty state
        if (this.quickActions.length === 0) {
            const emptyEl = document.createElement('div');
            emptyEl.className = 'empty';
            if (this.canManageQuickActions) {
                emptyEl.textContent = i18n.qa_empty_manage || 'No quick actions yet. Create one in Settings.';
            } else {
                emptyEl.textContent = i18n.qa_empty || 'No quick actions yet.';
            }
            this.quickActionsList.appendChild(emptyEl);
        } else {
            this.quickActions.forEach((action) => {
                const btn = document.createElement('button');
                btn.type = 'button';
                btn.className = 'quick-action-item';

                const labelEl = document.createElement('span');
                labelEl.className = 'quick-action-label';
                labelEl.textContent = action.label;

                if (action.mode === 'fill') {
                    labelEl.insertAdjacentHTML('beforeend', '<svg class="quick-action-mode-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/></svg>');
                }

                const previewEl = document.createElement('span');
                previewEl.className = 'quick-action-preview';
                previewEl.textContent = action.message;

                btn.appendChild(labelEl);
                btn.appendChild(previewEl);
                btn.addEventListener('click', () => this.handleQuickAction(action));

                this.quickActionsList.appendChild(btn);
            });
        }

        this.quickActionsBtn.addEventListener('click', () => this.openQuickActions());
        this.quickActionsClose.addEventListener('click', () => this.closeQuickActions());

        // Escape key to close overlay
        this.handleQuickActionsKeydown = (e) => {
            if (e.key === 'Escape' && !this.quickActionsOverlay.classList.contains('hidden')) {
                this.closeQuickActions();
            }
        };
        document.addEventListener('keydown', this.handleQuickActionsKeydown);

        // Clean up on HTMX body swap (prevents listener leak when navigating away)
        this.handleBeforeSwap = () => this.cleanup();
        document.addEventListener('htmx:beforeSwap', this.handleBeforeSwap, { once: true });
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
        this.closeQuickActions();
        if (action.mode === 'fill') {
            this.input.value = action.message;
            this.input.focus();
        } else {
            this.wsContainer.send({ content: action.message, topic_id: this.topicId });
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
        const msgEl = this._buildMessageEl(msg);
        this.messagesEl.insertBefore(msgEl, this.messagesEl.firstChild);
        this._updateGroupingOnPrepend(msgEl);
    }

};

var container = document.querySelector('[data-page="topic-chat"]');
if (container) {
    var topicId = parseInt(container.dataset.topicId, 10);
    window.topicChatClient = new TopicChatClient(topicId);
}
