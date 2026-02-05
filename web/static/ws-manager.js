// WebSocket manager - handles connection lifecycle and event dispatching
(function() {
    const container = document.getElementById('ws-connection');
    if (!container) return;

    if (container.dataset.initialized === 'true') return;
    container.dataset.initialized = 'true';

    let ws = null;
    let reconnectAttempts = 0;
    const MAX_RECONNECT_DELAY = 30000;

    function connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/chat`;

        ws = new WebSocket(wsUrl);
        container._ws = ws;

        ws.onopen = () => {
            console.log('WebSocket connected');
            reconnectAttempts = 0;
            dispatchStatus('connected');
        };

        ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            dispatchMessage(data);
        };

        ws.onclose = (event) => {
            console.log('WebSocket disconnected, code:', event.code);
            dispatchStatus('disconnected');

            // Auth error - redirect to login
            if (event.code === 1008 || event.code === 4001) {
                window.location.href = '/';
                return;
            }

            // Normal closure - intentional, don't reconnect
            if (event.code === 1000) {
                return;
            }

            scheduleReconnect();
        };

        ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            dispatchStatus('error', error);
        };
    }

    function dispatchStatus(status, detail = null) {
        document.dispatchEvent(new CustomEvent('bobot:connection-status', {
            detail: { status, detail }
        }));
    }

    function dispatchMessage(data) {
        if (data.topic_id) {
            document.dispatchEvent(new CustomEvent('bobot:topic-message', {
                detail: data
            }));
        } else {
            document.dispatchEvent(new CustomEvent('bobot:chat-message', {
                detail: data
            }));
        }
    }

    function scheduleReconnect() {
        const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), MAX_RECONNECT_DELAY);
        reconnectAttempts++;
        console.log(`Reconnecting in ${delay}ms...`);
        setTimeout(connect, delay);
    }

    container.connect = function() {
        if (ws && ws.readyState === WebSocket.OPEN) {
            return;
        }
        connect();
    };

    container.send = function(message) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify(message));
            return true;
        }
        return false;
    };

    container.close = function() {
        if (ws) {
            ws.close(1000);
            ws = null;
        }
    };

    // Close WebSocket on logout
    document.addEventListener('bobot:logout', function() {
        container.close();
    });
})();
