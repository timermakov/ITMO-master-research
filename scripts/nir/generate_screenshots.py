#!/usr/bin/env python3
"""Generate NIR screenshot PNGs for docs/nir/images/screenshots/."""

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "docs" / "nir" / "images" / "screenshots"


def _font(size: int):
    for name in ("Menlo.ttc", "DejaVuSansMono.ttf", "LiberationMono-Regular.ttf"):
        try:
            return ImageFont.truetype(name, size)
        except OSError:
            continue
    return ImageFont.load_default()


def placeholder(title: str, subtitle: str, path: Path, size=(1200, 700)):
    img = Image.new("RGB", size, (245, 247, 250))
    draw = ImageDraw.Draw(img)
    draw.rectangle((40, 40, size[0] - 40, size[1] - 40), outline=(180, 190, 200), width=2)
    title_font = _font(36)
    sub_font = _font(22)
    draw.text((80, 120), title, fill=(30, 40, 55), font=title_font)
    draw.text((80, 200), subtitle, fill=(80, 90, 105), font=sub_font)
    draw.text(
        (80, 280),
        "Заменить на скриншот из my.itmo перед сдачей отчёта.",
        fill=(120, 130, 145),
        font=sub_font,
    )
    img.save(path)


def terminal(lines: list[str], path: Path, width=1400):
    font = _font(18)
    line_h = 24
    pad = 24
    height = pad * 2 + line_h * len(lines)
    img = Image.new("RGB", (width, height), (24, 26, 30))
    draw = ImageDraw.Draw(img)
    y = pad
    for line in lines:
        draw.text((pad, y), line, fill=(220, 225, 230), font=font)
        y += line_h
    img.save(path)


def main():
    OUT.mkdir(parents=True, exist_ok=True)

    placeholder(
        "my.itmo — индивидуальное задание НИР 2",
        "Модуль «Практика», тема: разработка системы динамического прогрева сервисов",
        OUT / "01_my_itmo_assignment.png",
    )
    placeholder(
        "my.itmo — утверждение задания",
        "Уведомление об утверждении индивидуального задания научным руководителем",
        OUT / "02_my_itmo_approved.png",
    )

    docker_ps = [
        "$ docker compose -f deploy/docker-compose.yml ps",
        "NAME                      IMAGE                    STATUS          PORTS",
        "zookeeper                 zookeeper:3.9            Up 2 minutes    2181/tcp",
        "loaded-service-active     dwss-loaded-service      Up 2 minutes    8080/tcp",
        "loaded-service-warmup     dwss-loaded-service      Up 2 minutes    8081/tcp",
        "mirror-proxy              dwss-mirror-proxy        Up 2 minutes    8090->8090/tcp",
        "warmup-coordinator        dwss-warmup-coordinator  Up 2 minutes    8091->8091/tcp",
    ]
    terminal(docker_ps, OUT / "03_docker_compose_ps.png")

    bench_lines = [
        "$ make bench-all",
        "cd benchmark && go run ./cmd/bench run --scenarios all --out ../results/full",
        "scenario s0-control: runs=20 filtered=25 cv=16.3%",
        "scenario s-ref:      runs=20 filtered=20 cv=13.1%",
        "scenario s-dw:       runs=20 filtered=20 cv=18.6%",
        "T_first P50: S0=14.67 ms  S_ref=2.02 ms  S_dw=3.97 ms",
        "hypotheses:",
        "  H1:     pass  (P50 ratio S_dw/S0 = 0.271 <= 0.5)",
        "  H2:     pass  (P50 ratio S_dw/S_ref = 1.964 <= 2.0)",
        "  H2_CI:  pass",
        "  H3:     pass  (readyz=200 во всех валидных прогонах)",
        "wrote ../results/full/results.json",
    ]
    terminal(bench_lines, OUT / "04_bench_all_output.png")

    print(f"Wrote screenshots to {OUT}")


if __name__ == "__main__":
    main()
