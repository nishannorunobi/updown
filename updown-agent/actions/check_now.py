# check_now.py — the page's "⟳ Check now" button ([2.7]): updown looks at the phone's network NOW instead of at
# its next check (every CHECK_SECONDS) — e.g. right after joining the phone's hotspot.
# Called by updown-agent (actions.conf) as run(ctx, value); returns the line the page shows.


def run(ctx, value=None):
    c = ctx.updown("/host/api/check")["connection"]
    if c.get("state") == "none":
        return "checked — no phone network: " + (c.get("reason") or "?")
    return f"checked — {c.get('type') or c.get('via')} on {c.get('iface') or '?'}, this PC is {c.get('pc')}"
