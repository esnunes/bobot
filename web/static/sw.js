// Service worker for Web Push notifications only.
// No fetch event handling — HTMX and WebSocket traffic is untouched.

self.addEventListener("push", function (event) {
  if (!event.data) return;

  var payload;
  try {
    payload = event.data.json();
  } catch (e) {
    return;
  }

  var options = {
    body: payload.body || "",
    icon: "/static/icon-192x192.png",
    tag: payload.tag || "bobot",
    data: { url: payload.url || "/" },
  };

  event.waitUntil(
    self.registration.showNotification(payload.title || "Bobot", options)
  );
});

self.addEventListener("notificationclick", function (event) {
  event.notification.close();

  var url = (event.notification.data && event.notification.data.url) || "/";

  event.waitUntil(
    self.clients
      .matchAll({ type: "window", includeUncontrolled: true })
      .then(function (clientList) {
        // Try to focus an existing window
        for (var i = 0; i < clientList.length; i++) {
          var client = clientList[i];
          if ("focus" in client) {
            client.focus();
            client.postMessage({ type: "navigate", url: url });
            return;
          }
        }
        // No existing window — open a new one with query param
        return self.clients.openWindow("/?navigate=" + encodeURIComponent(url));
      })
  );
});
