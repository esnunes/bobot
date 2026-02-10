window.MessageRenderer = {
    renderMessageContent(content, role) {
        if (role !== 'assistant' && role !== 'system') {
            return null;
        }

        if (typeof marked === 'undefined' || typeof DOMPurify === 'undefined') {
            return null;
        }

        var html = marked.parse(content, { breaks: true });
        return DOMPurify.sanitize(html, {
            ADD_TAGS: ['bobot'],
            ADD_ATTR: ['label', 'action', 'message', 'confirm']
        });
    },

    highlightCodeBlocks(parentEl) {
        if (typeof hljs === 'undefined') {
            return;
        }
        parentEl.querySelectorAll('pre code').forEach(function(block) {
            hljs.highlightElement(block);
        });
    },

    processBobotTags(parentEl, sendFn, isHistory) {
        var bobotEls = parentEl.querySelectorAll('bobot');
        bobotEls.forEach(function(el) {
            var label = el.getAttribute('label');
            var action = el.getAttribute('action');
            var message = el.getAttribute('message');
            var needsConfirm = el.hasAttribute('confirm');

            // Validate: remove invalid tags
            if (!label || action !== 'send-message' || !message) {
                el.remove();
                return;
            }

            var btn = document.createElement('button');
            btn.className = 'bobot-action-btn';
            btn.textContent = label;
            btn.setAttribute('data-action', action);
            btn.setAttribute('data-message', message);
            if (needsConfirm) {
                btn.setAttribute('data-confirm', '');
            }
            btn.setAttribute('data-original-label', label);

            if (isHistory) {
                btn.disabled = true;
            } else {
                btn.addEventListener('click', function(e) {
                    e.stopPropagation();

                    if (needsConfirm && !btn.classList.contains('bobot-action-btn--confirming')) {
                        // First click: enter confirm state
                        MessageRenderer.resetConfirmingButtons();
                        btn.classList.add('bobot-action-btn--confirming');
                        btn.textContent = 'Confirm?';
                        return;
                    }

                    // Execute action
                    btn.disabled = true;
                    btn.classList.remove('bobot-action-btn--confirming');
                    btn.textContent = btn.getAttribute('data-original-label');
                    sendFn(message);
                });
            }

            el.replaceWith(btn);
        });
    },

    resetConfirmingButtons() {
        document.querySelectorAll('.bobot-action-btn--confirming').forEach(function(btn) {
            btn.classList.remove('bobot-action-btn--confirming');
            btn.textContent = btn.getAttribute('data-original-label');
        });
    }
};
