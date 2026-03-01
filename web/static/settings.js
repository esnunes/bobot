// Settings page interactions.
// Handles display name save, auto-read toggle, delete/leave topic.
// Push notification and mute toggles are handled by push.js (global).

(function() {
    'use strict';

    var i18nEl = document.querySelector('script[data-i18n]');
    var i18n = i18nEl ? JSON.parse(i18nEl.textContent) : {};

    var container = document.querySelector('[data-page="settings"]');
    if (!container) return;

    var topicId = container.dataset.topicId ? parseInt(container.dataset.topicId, 10) : null;
    var currentUserId = container.dataset.currentUserId ? parseInt(container.dataset.currentUserId, 10) : null;

    // Display name form
    var displayNameForm = document.getElementById('display-name-form');
    if (displayNameForm) {
        displayNameForm.addEventListener('submit', function(e) {
            e.preventDefault();
            var input = document.getElementById('display-name-input');
            var savedMsg = document.getElementById('display-name-saved');
            var submitBtn = displayNameForm.querySelector('button[type="submit"]');
            var name = input.value.trim();
            if (!name) return;

            submitBtn.disabled = true;
            fetch('/api/user/display-name', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                body: 'display_name=' + encodeURIComponent(name)
            })
            .then(function(resp) {
                if (resp.ok) {
                    savedMsg.style.display = '';
                    savedMsg.style.opacity = '1';
                    setTimeout(function() {
                        savedMsg.style.opacity = '0';
                        setTimeout(function() { savedMsg.style.display = 'none'; }, 300);
                    }, 2000);
                } else {
                    console.error('Failed to update display name:', resp.status);
                }
            })
            .catch(function(err) {
                console.error('Failed to update display name:', err);
            })
            .finally(function() {
                submitBtn.disabled = false;
            });
        });
    }

    // Email form
    var emailForm = document.getElementById('email-form');
    if (emailForm) {
        emailForm.addEventListener('submit', function(e) {
            e.preventDefault();
            var input = document.getElementById('email-input');
            var savedMsg = document.getElementById('email-saved');
            var preview = document.getElementById('gravatar-preview');
            var submitBtn = emailForm.querySelector('button[type="submit"]');
            var email = input.value.trim();

            submitBtn.disabled = true;
            fetch('/api/user/email', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                body: 'email=' + encodeURIComponent(email)
            })
            .then(function(resp) {
                if (resp.ok) {
                    return resp.json().then(function(data) {
                        if (preview && data.gravatar_url) {
                            preview.src = data.gravatar_url;
                        }
                        savedMsg.style.display = '';
                        savedMsg.style.opacity = '1';
                        setTimeout(function() {
                            savedMsg.style.opacity = '0';
                            setTimeout(function() { savedMsg.style.display = 'none'; }, 300);
                        }, 2000);
                    });
                } else {
                    console.error('Failed to update email:', resp.status);
                }
            })
            .catch(function(err) {
                console.error('Failed to update email:', err);
            })
            .finally(function() {
                submitBtn.disabled = false;
            });
        });
    }

    // Auto-read toggle
    var autoReadBtn = container.querySelector('[data-auto-read-toggle]');
    if (autoReadBtn) {
        autoReadBtn.addEventListener('click', function() {
            var isAutoRead = autoReadBtn.getAttribute('data-auto-read') === 'true';
            var method = isAutoRead ? 'DELETE' : 'POST';
            autoReadBtn.disabled = true;
            fetch('/api/topics/' + topicId + '/auto-read', { method: method })
                .then(function(resp) {
                    if (resp.ok) {
                        var newState = !isAutoRead;
                        autoReadBtn.setAttribute('data-auto-read', String(newState));
                        autoReadBtn.setAttribute('aria-checked', String(newState));
                        if (newState) {
                            document.dispatchEvent(new CustomEvent('bobot:chat-read', {
                                detail: { topic_id: topicId }
                            }));
                        }
                    }
                })
                .catch(function(err) { console.error('Auto-read toggle failed:', err); })
                .finally(function() { autoReadBtn.disabled = false; });
        });
    }

    // Auto-respond toggle
    var autoRespondBtn = container.querySelector('[data-auto-respond-toggle]');
    if (autoRespondBtn) {
        autoRespondBtn.addEventListener('click', function() {
            var isAutoRespond = autoRespondBtn.getAttribute('data-auto-respond') === 'true';
            var method = isAutoRespond ? 'DELETE' : 'POST';
            autoRespondBtn.disabled = true;
            fetch('/api/topics/' + topicId + '/auto-respond', { method: method })
                .then(function(resp) {
                    if (resp.ok) {
                        var newState = !isAutoRespond;
                        autoRespondBtn.setAttribute('data-auto-respond', String(newState));
                        autoRespondBtn.setAttribute('aria-checked', String(newState));
                    }
                })
                .catch(function(err) { console.error('Auto-respond toggle failed:', err); })
                .finally(function() { autoRespondBtn.disabled = false; });
        });
    }

    // Delete topic
    var deleteBtn = document.getElementById('delete-btn');
    if (deleteBtn && topicId) {
        deleteBtn.addEventListener('click', function() {
            if (!confirm(i18n.confirm_delete_topic || 'Are you sure you want to delete this topic?')) return;
            fetch('/api/topics/' + topicId, { method: 'DELETE' })
                .then(function(resp) {
                    if (!resp.ok) throw new Error('Failed to delete topic');
                    htmx.ajax('GET', '/chats', { target: 'body', swap: 'innerHTML' });
                })
                .catch(function(err) {
                    console.error('Failed to delete topic:', err);
                    alert(i18n.error_delete_topic || 'Failed to delete topic');
                });
        });
    }

    // Calendar disconnect
    var calDisconnectBtn = container.querySelector('[data-calendar-disconnect]');
    if (calDisconnectBtn) {
        calDisconnectBtn.addEventListener('click', function() {
            var calTopicId = calDisconnectBtn.getAttribute('data-topic-id');
            calDisconnectBtn.disabled = true;
            fetch('/api/calendar?topic_id=' + calTopicId, { method: 'DELETE' })
                .then(function(resp) {
                    if (resp.ok) {
                        htmx.ajax('GET', '/settings?topic_id=' + calTopicId, { target: 'body', swap: 'innerHTML' });
                    } else {
                        console.error('Failed to disconnect calendar:', resp.status);
                    }
                })
                .catch(function(err) { console.error('Calendar disconnect failed:', err); })
                .finally(function() { calDisconnectBtn.disabled = false; });
        });
    }

    // Spotify link
    var spotifyLinkBtn = container.querySelector('[data-spotify-link]');
    if (spotifyLinkBtn) {
        spotifyLinkBtn.addEventListener('click', function() {
            var spotifyTopicId = spotifyLinkBtn.getAttribute('data-topic-id');
            spotifyLinkBtn.disabled = true;
            fetch('/api/spotify/link?topic_id=' + spotifyTopicId, { method: 'POST' })
                .then(function(resp) {
                    if (resp.ok) {
                        htmx.ajax('GET', '/settings?topic_id=' + spotifyTopicId, { target: 'body', swap: 'innerHTML' });
                    } else {
                        console.error('Failed to link Spotify:', resp.status);
                    }
                })
                .catch(function(err) { console.error('Spotify link failed:', err); })
                .finally(function() { spotifyLinkBtn.disabled = false; });
        });
    }

    // Spotify unlink
    var spotifyUnlinkBtn = container.querySelector('[data-spotify-unlink]');
    if (spotifyUnlinkBtn) {
        spotifyUnlinkBtn.addEventListener('click', function() {
            var spotifyTopicId = spotifyUnlinkBtn.getAttribute('data-topic-id');
            spotifyUnlinkBtn.disabled = true;
            fetch('/api/spotify/link?topic_id=' + spotifyTopicId, { method: 'DELETE' })
                .then(function(resp) {
                    if (resp.ok) {
                        htmx.ajax('GET', '/settings?topic_id=' + spotifyTopicId, { target: 'body', swap: 'innerHTML' });
                    } else {
                        console.error('Failed to unlink Spotify:', resp.status);
                    }
                })
                .catch(function(err) { console.error('Spotify unlink failed:', err); })
                .finally(function() { spotifyUnlinkBtn.disabled = false; });
        });
    }

    // Leave topic
    var leaveBtn = document.getElementById('leave-btn');
    if (leaveBtn && topicId && currentUserId) {
        leaveBtn.addEventListener('click', function() {
            if (!confirm(i18n.confirm_leave_topic || 'Are you sure you want to leave this topic?')) return;
            fetch('/api/topics/' + topicId + '/members/' + currentUserId, { method: 'DELETE' })
                .then(function(resp) {
                    if (!resp.ok) throw new Error('Failed to leave topic');
                    htmx.ajax('GET', '/chats', { target: 'body', swap: 'innerHTML' });
                })
                .catch(function(err) {
                    console.error('Failed to leave topic:', err);
                    alert(i18n.error_leave_topic || 'Failed to leave topic');
                });
        });
    }
})();
