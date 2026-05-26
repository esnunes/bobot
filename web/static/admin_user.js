// Admin user detail page interactions.
// Handles the admin "change password" form: client-side confirmation match,
// a confirm dialog warning that the user's sessions will be revoked, and the
// POST to /admin/users/{id}/password.

(function() {
    'use strict';

    var i18nEl = document.querySelector('script[data-i18n]');
    var i18n = i18nEl ? JSON.parse(i18nEl.textContent) : {};

    var form = document.getElementById('admin-password-form');
    if (!form) return;

    var userId = form.dataset.userId;
    var isSelf = form.dataset.selfReset === 'true';
    var userName = form.dataset.userName || '';

    var passwordInput = document.getElementById('admin-password-input');
    var confirmInput = document.getElementById('admin-password-confirm');
    var msg = document.getElementById('admin-password-msg');
    var submitBtn = form.querySelector('button[type="submit"]');

    function showMessage(text, isError) {
        msg.textContent = text;
        msg.classList.toggle('admin-password-msg--error', !!isError);
    }

    form.addEventListener('submit', function(e) {
        e.preventDefault();

        var password = passwordInput.value;
        var confirm = confirmInput.value;

        if (password !== confirm) {
            showMessage(i18n.pw_mismatch || 'Passwords do not match', true);
            return;
        }

        var prompt = isSelf
            ? (i18n.pw_self_confirm || 'Change your own password? You will be signed out on your next session refresh.')
            : (i18n.pw_confirm || 'Change the password for {name}? They will be signed out on their next session refresh.').replace('{name}', userName);
        if (!window.confirm(prompt)) return;

        submitBtn.disabled = true;
        showMessage('', false);

        fetch('/admin/users/' + userId + '/password', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: 'password=' + encodeURIComponent(password) +
                  '&confirm_password=' + encodeURIComponent(confirm)
        })
        .then(function(resp) {
            if (resp.ok) {
                form.reset();
                showMessage(i18n.pw_success || 'Password changed.', false);
                return;
            }
            return resp.text().then(function(text) {
                showMessage(text || i18n.pw_error || 'Could not change password.', true);
            });
        })
        .catch(function() {
            showMessage(i18n.pw_error || 'Could not change password.', true);
        })
        .finally(function() {
            submitBtn.disabled = false;
        });
    });
})();
