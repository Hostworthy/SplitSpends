"""Composes docs/images/hero.webp from a desktop and a phone screenshot.

Usage: python3 hero.py <desktop.png> <phone.png> <out.webp>
Use capture.js output, for example out/desktop-group-expenses.png and
out/phone-group-balances.png.
"""
import sys

from PIL import Image, ImageDraw, ImageFilter

W, H = 2400, 1380


def rounded(img, radius):
    mask = Image.new("L", img.size, 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, img.size[0] - 1, img.size[1] - 1), radius, fill=255)
    out = img.convert("RGBA")
    out.putalpha(mask)
    return out


def shadow(canvas, box, radius, blur=40, offset=(0, 24), alpha=70):
    layer = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
    x0, y0, x1, y1 = box
    ImageDraw.Draw(layer).rounded_rectangle(
        (x0 + offset[0], y0 + offset[1], x1 + offset[0], y1 + offset[1]), radius, fill=(16, 60, 52, alpha))
    canvas.alpha_composite(layer.filter(ImageFilter.GaussianBlur(blur)))


def background():
    top, bottom = (222, 242, 237), (245, 249, 248)
    grad = Image.new("RGB", (1, H))
    for y in range(H):
        t = y / H
        grad.putpixel((0, y), tuple(int(top[i] * (1 - t) + bottom[i] * t) for i in range(3)))
    bg = grad.resize((W, H))
    glow = Image.new("L", (W, H), 0)
    ImageDraw.Draw(glow).ellipse((1300, -500, 2900, 900), fill=120)
    glow = glow.filter(ImageFilter.GaussianBlur(220))
    return Image.composite(Image.new("RGB", (W, H), (164, 232, 214)), bg, glow).convert("RGBA")


def main(desktop_path, phone_path, out_path):
    canvas = background()

    desk = Image.open(desktop_path).convert("RGB")
    dw = 1760
    dh = int(desk.height * dw / desk.width)
    desk = desk.resize((dw, dh), Image.LANCZOS)
    bar = 52
    window = Image.new("RGB", (dw, dh + bar), (238, 243, 241))
    draw = ImageDraw.Draw(window)
    for i, color in enumerate([(237, 106, 94), (245, 191, 79), (98, 197, 84)]):
        draw.ellipse((24 + i * 32, 17, 42 + i * 32, 35), fill=color)
    draw.rounded_rectangle((dw // 2 - 260, 12, dw // 2 + 260, 40), 14, fill=(255, 255, 255))
    window.paste(desk, (0, bar))
    window = rounded(window, 22)
    dx, dy = 110, 120
    shadow(canvas, (dx, dy, dx + dw, dy + dh + bar), 22)
    canvas.alpha_composite(window, (dx, dy))

    phone = Image.open(phone_path).convert("RGB")
    ph = 1160
    pw = int(phone.width * ph / phone.height)
    phone = phone.resize((pw, ph), Image.LANCZOS)
    bezel = 16
    body = Image.new("RGB", (pw + 2 * bezel, ph + 2 * bezel), (26, 32, 30))
    screen = rounded(phone, 44)
    body.paste(screen, (bezel, bezel), screen)
    body = rounded(body, 58)
    px, py = W - body.width - 120, H - body.height - 60
    shadow(canvas, (px, py, px + body.width, py + body.height), 58, blur=46, alpha=95)
    canvas.alpha_composite(body, (px, py))

    out = canvas.convert("RGB")
    out = out.resize((1800, int(out.height * 1800 / out.width)), Image.LANCZOS)
    out.save(out_path, quality=90, method=6)


if __name__ == "__main__":
    main(*sys.argv[1:4])
