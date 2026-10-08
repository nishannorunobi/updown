# network_mode.py — the page's network switch ([2.7]): 📶 Hotspot only · 🌐 Any network. It changes BIND in
# updown.conf (the same line you can edit by hand) and restarts updown with it — a new link, new QR codes.
#     hotspot   only the phone's own network (Wi-Fi hotspot, USB, Bluetooth) — files never cross a foreign Wi-Fi
#     auto      any network this PC is on (only on a Wi-Fi you trust)
# Called by updown-agent (actions.conf): options(ctx) → the switch's choices, run(ctx, value) → the line the page shows.

CHOICES = [("hotspot", "📶", "Hotspot only"), ("auto", "🌐", "Any network")]


def options(ctx):
    now = ctx.setting("BIND", "hotspot")
    return [{"value": v, "icon": i, "label": l, "on": v == now} for v, i, l in CHOICES]


def run(ctx, value=None):
    if value not in dict((v, l) for v, _, l in CHOICES):
        raise ValueError(f"unknown network mode {value!r} — possible: hotspot, auto")
    if ctx.setting("BIND", "hotspot") == value:
        return f"already BIND={value}"
    ctx.set_setting("BIND", value)
    ctx.restart_updown(f"BIND={value}")
    return f"BIND={value} saved in updown.conf — updown restarts with it (new QR codes in a few seconds)"
