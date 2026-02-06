window.MessageRenderer = {
    renderMessageContent(content, role) {
        if (role !== 'assistant' && role !== 'system') {
            return null;
        }

        if (typeof marked === 'undefined' || typeof DOMPurify === 'undefined') {
            return null;
        }

        var html = marked.parse(content, { breaks: true });
        return DOMPurify.sanitize(html);
    },

    highlightCodeBlocks(parentEl) {
        if (typeof hljs === 'undefined') {
            return;
        }
        parentEl.querySelectorAll('pre code').forEach(function(block) {
            hljs.highlightElement(block);
        });
    }
};
