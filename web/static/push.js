// Push notification manager.
// Registers the service worker, handles subscribe/unsubscribe, and navigation from notification clicks.

(function () {
  "use strict";

  var vapidMeta = document.querySelector('meta[name="vapid-key"]');
  if (!vapidMeta) return; // Push not configured

  var vapidKey = vapidMeta.getAttribute("content");
  if (!vapidKey) return;

  if (!("serviceWorker" in navigator) || !("PushManager" in window)) return;

  // Register service worker
  navigator.serviceWorker
    .register("/sw.js")
    .then(function () {
      // Update notification button state once ready
      navigator.serviceWorker.ready.then(function () {
        updateButtons();
      });
    })
    .catch(function (err) {
      console.error("SW registration failed:", err);
    });

  // Listen for navigation messages from service worker (notification click)
  navigator.serviceWorker.addEventListener("message", function (event) {
    if (event.data && event.data.type === "navigate" && event.data.url) {
      navigateTo(event.data.url);
    }
  });

  // Listen for logout event
  document.body.addEventListener("bobot:logout", function () {
    disablePush();
  });

  // Re-initialize buttons after HTMX page swaps
  document.body.addEventListener("htmx:afterSettle", function () {
    updateButtons();
  });

  // Expose for button onclick
  window.bobotPush = {
    enable: enablePush,
    disable: disablePush,
    isEnabled: isEnabled,
    updateButtons: updateButtons,
  };

  function enablePush() {
    return navigator.serviceWorker.ready
      .then(function (reg) {
        return reg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: urlBase64ToUint8Array(vapidKey),
        });
      })
      .then(function (sub) {
        var key = sub.getKey("p256dh");
        var auth = sub.getKey("auth");
        return fetch("/api/push/subscribe", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            endpoint: sub.endpoint,
            keys: {
              p256dh: arrayBufferToBase64url(key),
              auth: arrayBufferToBase64url(auth),
            },
          }),
        });
      })
      .then(function () {
        updateButtons();
      })
      .catch(function (err) {
        if (Notification.permission === "denied") {
          alert(
            "Notifications are blocked. Please enable them in your browser settings."
          );
        }
        console.error("Push subscribe failed:", err);
      });
  }

  function disablePush() {
    return navigator.serviceWorker.ready
      .then(function (reg) {
        return reg.pushManager.getSubscription();
      })
      .then(function (sub) {
        if (!sub) return;
        var endpoint = sub.endpoint;
        return sub.unsubscribe().then(function () {
          return fetch("/api/push/subscribe", {
            method: "DELETE",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ endpoint: endpoint }),
          });
        });
      })
      .then(function () {
        updateButtons();
      })
      .catch(function (err) {
        console.error("Push unsubscribe failed:", err);
      });
  }

  function isEnabled() {
    return navigator.serviceWorker.ready
      .then(function (reg) {
        return reg.pushManager.getSubscription();
      })
      .then(function (sub) {
        return !!sub;
      });
  }

  function updateButtons() {
    isEnabled().then(function (enabled) {
      var buttons = document.querySelectorAll("[data-push-toggle]");
      buttons.forEach(function (btn) {
        btn.textContent = enabled
          ? "Disable notifications"
          : "Enable notifications";
        btn.onclick = function () {
          if (enabled) {
            disablePush();
          } else {
            enablePush();
          }
        };
        btn.style.display = "";
      });
    });
  }

  function navigateTo(url) {
    // Validate path — only allow /chat and /topics/{id}
    if (url !== "/chat" && !/^\/topics\/\d+$/.test(url)) return;

    if (typeof htmx === "undefined") {
      console.log("htmx not loaded");
      return;
    }
    htmx.ajax("GET", url, { target: "body" });
  }

  function urlBase64ToUint8Array(base64String) {
    var padding = "=".repeat((4 - (base64String.length % 4)) % 4);
    var base64 = (base64String + padding)
      .replace(/-/g, "+")
      .replace(/_/g, "/");
    var raw = atob(base64);
    var arr = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) {
      arr[i] = raw.charCodeAt(i);
    }
    return arr;
  }

  function arrayBufferToBase64url(buffer) {
    var bytes = new Uint8Array(buffer);
    var str = "";
    for (var i = 0; i < bytes.length; i++) {
      str += String.fromCharCode(bytes[i]);
    }
    return btoa(str).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }
})();
