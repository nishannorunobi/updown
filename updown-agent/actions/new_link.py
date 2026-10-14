# new_link.py — the page's "🔄 Refresh QR code" button ([2.7]): updown hands out a new token, so the page shows NEW QR
# codes and the old ones (a photo of the screen, a link sent by mistake) stop working at once.
# Called by updown-agent (actions.conf) as run(ctx, value); returns the line the page shows.


def run(ctx, value=None):
    ctx.updown("/host/api/new-link")
    return "QR codes refreshed — the page shows new ones; the old ones no longer work"
