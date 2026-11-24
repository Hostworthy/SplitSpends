#!/usr/bin/env python3
"""Renders every SplitSpends icon, splash and banner from one design: two
overlapping coins (the in-app logo) on the brand teal. Run from the
repository root after changing the design; the outputs are committed.

    python3 scripts/brand/render.py

Needs Pillow. Text uses the Inter font bundled in packages/client.
"""
import json
import os
from PIL import Image, ImageDraw, ImageFilter, ImageFont

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
FONTS = os.path.join(ROOT, "packages/client/assets/fonts")

TEAL_TOP = (18, 144, 122)      # #12907A
TEAL_BOTTOM = (10, 94, 78)     # #0A5E4E
BRAND = "#0E7C66"
LIGHT_BG = "#F5FAF7"
COIN_A = (255, 255, 255)
COIN_A_RING = (214, 239, 230)
COIN_B = (157, 235, 207)       # #9DEBCF
COIN_B_RING = (111, 211, 178)

SS = 4  # supersampling factor


def gradient(w, h):
    """Diagonal teal gradient, top-left light to bottom-right dark."""
    small = Image.new("RGB", (64, 64))
    px = small.load()
    for y in range(64):
        for x in range(64):
            t = (x + y) / 126
            px[x, y] = tuple(round(a + (b - a) * t) for a, b in zip(TEAL_TOP, TEAL_BOTTOM))
    return small.resize((w, h), Image.BILINEAR)


def disc(size, cx, cy, r):
    m = Image.new("L", (size, size), 0)
    ImageDraw.Draw(m).ellipse((cx - r, cy - r, cx + r, cy + r), fill=255)
    return m


def coins(size, scale=1.0, detail=True, mono=False):
    """The two-coin mark on a transparent canvas of size x size (already
    supersampled). scale 1.0 is the full-icon layout."""
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    u = size / 1024
    r = 232 * u * scale
    gap = 40 * u * scale
    cy = size / 2
    left = (size / 2 - 120 * u * scale, cy)
    right = (size / 2 + 120 * u * scale, cy)

    # The left coin, cut where the right coin (plus a gap) covers it.
    a = disc(size, *left, r)
    cut = disc(size, *right, r + gap)
    a = Image.composite(Image.new("L", (size, size), 0), a, cut)
    b = disc(size, *right, r)

    def paint(mask, fill, ring, center):
        layer = Image.new("RGBA", (size, size), (*fill, 255) if not mono else (255, 255, 255, 255))
        if detail and not mono:
            d = ImageDraw.Draw(layer)
            rr = r * 0.72
            w = max(1, round(r * 0.07))
            d.ellipse((center[0] - rr, center[1] - rr, center[0] + rr, center[1] + rr), outline=(*ring, 255), width=w)
        img.paste(layer, (0, 0), mask)

    paint(a, COIN_A, COIN_A_RING, left)
    paint(b, COIN_B, COIN_B_RING, right)
    return img


def icon(px, rounded=True, scale=1.0, detail=None, alpha=True):
    """The app icon at px x px. rounded gives the squircle-ish rounded square
    with transparent corners; otherwise the background fills the square
    (iOS and maskable icons, where the platform applies its own shape)."""
    if detail is None:
        detail = px >= 48
    size = px * SS
    bg = gradient(size, size).convert("RGBA")
    mark = coins(size, scale=scale, detail=detail)
    out = Image.alpha_composite(bg, mark)
    if rounded:
        mask = Image.new("L", (size, size), 0)
        ImageDraw.Draw(mask).rounded_rectangle((0, 0, size - 1, size - 1), radius=round(size * 0.225), fill=255)
        out.putalpha(mask)
    out = out.resize((px, px), Image.LANCZOS)
    return out if alpha else out.convert("RGB")


def foreground(px, mono=False):
    """Android adaptive-icon foreground: the mark inside the 66dp safe zone
    of a 108dp canvas, on transparency."""
    size = px * SS
    return coins(size, scale=0.86, detail=not mono, mono=mono).resize((px, px), Image.LANCZOS)


def save(img, *parts):
    path = os.path.join(ROOT, *parts)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    img.save(path, optimize=True)
    return path


def font(weight, px):
    return ImageFont.truetype(os.path.join(FONTS, f"Inter-{weight}.ttf"), px)


def banner(w, h, path_parts, tagline=True):
    """Social preview / store banner: icon, name and tagline on teal."""
    img = gradient(w, h).convert("RGBA")
    # Soft glow behind the icon.
    glow = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    s = round(h * 0.46)
    x0, y0 = round(w * 0.08), (h - s) // 2
    ImageDraw.Draw(glow).rounded_rectangle((x0 + 10, y0 + 18, x0 + s + 10, y0 + s + 18), radius=round(s * 0.225), fill=(0, 40, 32, 110))
    img = Image.alpha_composite(img, glow.filter(ImageFilter.GaussianBlur(24)))
    ic = icon(s)
    # A white edge so the teal icon stands out from the teal background.
    edge = Image.new("RGBA", (s + 8, s + 8), (0, 0, 0, 0))
    ImageDraw.Draw(edge).rounded_rectangle((0, 0, s + 7, s + 7), radius=round((s + 8) * 0.225), fill=(255, 255, 255, 60))
    img.alpha_composite(edge, (x0 - 4, y0 - 4))
    img.alpha_composite(ic, (x0, y0))

    d = ImageDraw.Draw(img)
    tx = x0 + s + round(w * 0.05)
    title = font("Bold", round(h * 0.15))
    d.text((tx, h * 0.29), "SplitSpends", font=title, fill=(255, 255, 255))
    if tagline:
        d.text((tx, h * 0.50), "Split expenses, not friendships.", font=font("SemiBold", round(h * 0.066)), fill=(215, 245, 234))
        d.text((tx, h * 0.62), "Self-hosted · Web, Android and iPhone", font=font("Regular", round(h * 0.046)), fill=(189, 232, 217))
        d.text((tx, h * 0.69), "Free for personal use", font=font("Regular", round(h * 0.046)), fill=(189, 232, 217))
    return save(img.convert("RGB"), *path_parts)


def main():
    written = []

    # Web: favicon, PWA icons (rounded and maskable), Apple touch icon.
    web = ("apps", "web", "web")
    written.append(save(icon(32), *web, "favicon.png"))
    written.append(save(icon(192), *web, "icons", "Icon-192.png"))
    written.append(save(icon(512), *web, "icons", "Icon-512.png"))
    written.append(save(icon(192, rounded=False), *web, "icons", "Icon-maskable-192.png"))
    written.append(save(icon(512, rounded=False), *web, "icons", "Icon-maskable-512.png"))
    written.append(save(icon(180, rounded=False, alpha=False), *web, "icons", "apple-touch-icon.png"))
    # Notification badge: browsers use only its alpha, so a white silhouette.
    written.append(save(coins(96 * SS, scale=1.25, detail=False, mono=True).resize((96, 96), Image.LANCZOS), *web, "icons", "badge-96.png"))

    # Android: legacy launcher icons, adaptive foreground and monochrome.
    res = ("apps", "apk", "android", "app", "src", "main", "res")
    for density, legacy, adaptive in [("mdpi", 48, 108), ("hdpi", 72, 162), ("xhdpi", 96, 216), ("xxhdpi", 144, 324), ("xxxhdpi", 192, 432)]:
        written.append(save(icon(legacy), *res, f"mipmap-{density}", "ic_launcher.png"))
        written.append(save(foreground(adaptive), *res, f"mipmap-{density}", "ic_launcher_foreground.png"))
        written.append(save(foreground(adaptive, mono=True), *res, f"mipmap-{density}", "ic_launcher_monochrome.png"))
        written.append(save(foreground(adaptive), *res, f"drawable-{density}", "launch_image.png"))
        # Android 12+ splash icons are 288dp, the mark inside the middle 192dp.
        splash = adaptive * 288 // 108
        written.append(save(foreground(splash), *res, f"drawable-{density}", "splash_icon.png"))

    # iOS: every size Contents.json lists, opaque and square (iOS rounds).
    appicon = os.path.join(ROOT, "apps/ios/ios/Runner/Assets.xcassets/AppIcon.appiconset")
    for item in json.load(open(os.path.join(appicon, "Contents.json")))["images"]:
        pt = float(item["size"].split("x")[0])
        px = round(pt * int(item["scale"][0]))
        written.append(save(icon(px, rounded=False, alpha=False), appicon, item["filename"]))
    launch = os.path.join(ROOT, "apps/ios/ios/Runner/Assets.xcassets/LaunchImage.imageset")
    for name, px in [("LaunchImage.png", 96), ("LaunchImage@2x.png", 192), ("LaunchImage@3x.png", 288)]:
        written.append(save(icon(px), launch, name))

    # Banners: social preview (GitHub recommends 1280x640, also used as
    # og:image) and the Google Play feature graphic (1024x500).
    written.append(banner(1280, 640, ("docs", "images", "banner.png")))
    written.append(banner(1280, 640, ("apps", "web", "web", "social.png")))
    written.append(banner(1024, 500, ("docs", "images", "feature-graphic.png")))

    for path in written:
        print(os.path.relpath(path, ROOT))


if __name__ == "__main__":
    main()
