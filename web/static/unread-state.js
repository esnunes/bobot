// Unread state manager - tracks which chats have unread messages
// Server renders initial indicators; this module handles real-time updates
// by dispatching 'bobot:unread-changed' when WebSocket events modify the set.
(function() {
    var unreadChatIds = new Set();

    function getCurrentChatId() {
        var chatPage = document.querySelector('[data-page="chat"]');
        if (chatPage) return 0;
        var topicPage = document.querySelector('[data-page="topic-chat"]');
        if (topicPage) return parseInt(topicPage.dataset.topicId, 10);
        return null;
    }

    function initFromServer() {
        var el = document.querySelector('script[data-unread-state]');
        if (el) {
            try {
                unreadChatIds = new Set(JSON.parse(el.textContent));
            } catch(e) {}
        }
        var currentId = getCurrentChatId();
        if (currentId !== null) {
            unreadChatIds.delete(currentId);
        }
    }

    function dispatchChanged() {
        document.dispatchEvent(new CustomEvent('bobot:unread-changed', {
            detail: { chatIds: unreadChatIds }
        }));
    }

    // WebSocket event handlers — only these trigger dispatches
    document.addEventListener('bobot:chat-message', function() {
        if (getCurrentChatId() !== 0) {
            unreadChatIds.add(0);
            dispatchChanged();
        }
    });

    document.addEventListener('bobot:topic-message', function(e) {
        var topicId = e.detail.topic_id;
        if (getCurrentChatId() !== topicId) {
            unreadChatIds.add(topicId);
            dispatchChanged();
        }
    });

    document.addEventListener('bobot:chat-read', function(e) {
        unreadChatIds.delete(e.detail.topic_id);
        dispatchChanged();
    });

    // Re-initialize set from server data after HTMX page swaps
    // (no dispatch — server already rendered the correct indicators)
    document.addEventListener('htmx:afterSettle', function() {
        initFromServer();
    });

    // Initial setup — populate set for real-time tracking
    initFromServer();
})();
