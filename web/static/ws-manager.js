// WebSocket manager - handles connection lifecycle and event dispatching
(function() {
    const container = document.getElementById('ws-connection');
    if (!container) return;

    // Prevent re-initialization on HTMX swaps
    if (container.dataset.initialized === 'true') return;
    container.dataset.initialized = 'true';

    let ws = null;
    let reconnectAttempts = 0;
    const MAX_RECONNECT_DELAY = 30000;

    function getToken() {
        return localStorage.getItem('access_token');
    }

    function connect() {
        const token = getToken();
        if (!token) {
            dispatchStatus('disconnected', 'no token');
            return;
        }

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/chat?token=${token}`;

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

        ws.onclose = () => {
            console.log('WebSocket disconnected');
            dispatchStatus('disconnected');
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
        if (data.group_id) {
            document.dispatchEvent(new CustomEvent('bobot:group-message', {
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
        setTimeout(() => refreshAndReconnect(), delay);
    }

    async function refreshAndReconnect() {
        const refreshToken = localStorage.getItem('refresh_token');
        if (!refreshToken) {
            dispatchStatus('auth-expired');
            document.dispatchEvent(new CustomEvent('bobot:auth-expired'));
            return;
        }

        try {
            const resp = await fetch('/api/refresh', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ refresh_token: refreshToken })
            });

            if (!resp.ok) {
                throw new Error('Refresh failed');
            }

            const data = await resp.json();
            localStorage.setItem('access_token', data.access_token);
            connect();
        } catch (err) {
            console.error('Token refresh failed:', err);
            dispatchStatus('auth-expired');
            document.dispatchEvent(new CustomEvent('bobot:auth-expired'));
        }
    }

    // Public API via container element
    container.send = function(message) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify(message));
            return true;
        }
        return false;
    };

    container.close = function() {
        if (ws) {
            ws.close();
            ws = null;
        }
    };

    container.reconnect = function() {
        if (ws) ws.close();
        reconnectAttempts = 0;
        connect();
    };

    // Handle auth expiration globally
    document.addEventListener('bobot:auth-expired', () => {
        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
        localStorage.removeItem('lastMessageTimestamp');
        window.location.href = '/';
    });

    // Initialize connection if we have a token
    if (getToken()) {
        connect();
    }

    // Reconnect when token changes (e.g., after login)
    document.addEventListener('bobot:token-updated', () => {
        container.reconnect();
    });
})();
