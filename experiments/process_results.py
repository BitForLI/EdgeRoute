#!/usr/bin/env python3
"""Build reproducible Day 6 CSV, Markdown, and comparison plots from raw k6 runs."""

from __future__ import annotations

import argparse
import csv
import json
import math
import statistics
from collections import defaultdict
from pathlib import Path

import matplotlib.pyplot as plt


def metric(summary: dict, name: str, value: str, default: float = 0.0) -> float:
    try:
        payload = summary["metrics"][name]
        if "values" in payload:
            return float(payload["values"][value])
        if value == "rate" and "value" in payload:
            return float(payload["value"])
        return float(payload[value])
    except (KeyError, TypeError, ValueError):
        return default


def load_runs(raw_root: Path) -> list[dict]:
    runs: list[dict] = []
    for metadata_path in sorted(raw_root.glob("*/experiment-metadata.json")):
        run_dir = metadata_path.parent
        summary_path = run_dir / "k6-summary.json"
        if not summary_path.exists():
            continue
        metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
        summary = json.loads(summary_path.read_text(encoding="utf-8"))
        playlist_requests = metric(summary, "hls_playlist_requests", "count")
        playlist_failures = metric(summary, "hls_playlist_failures", "count")
        segment_requests = metric(summary, "hls_segment_requests", "count")
        segment_failures = metric(summary, "hls_segment_failures", "count")
        runs.append(
            {
                "run_id": metadata["run_id"],
                "variant": metadata["variant"],
                "scenario": metadata["scenario"],
                "repetition": metadata["repetition"],
                "profile": metadata["profile"],
                "playlist_success_rate": 1.0 - playlist_failures / playlist_requests if playlist_requests else 0.0,
                "segment_success_rate": 1.0 - segment_failures / segment_requests if segment_requests else 0.0,
                "session_failure_rate": metric(summary, "hls_session_failures", "rate"),
                "segment_p50_ms": metric(summary, "hls_segment_duration", "med"),
                "segment_p95_ms": metric(summary, "hls_segment_duration", "p(95)"),
                "segment_p99_ms": metric(summary, "hls_segment_duration", "p(99)"),
                "iterations": metric(summary, "iterations", "count"),
            }
        )
    return runs


def percentile(values: list[float], fraction: float) -> float:
    if not values:
        return math.nan
    ordered = sorted(values)
    rank = (len(ordered) - 1) * fraction
    lower = math.floor(rank)
    upper = math.ceil(rank)
    if lower == upper:
        return ordered[lower]
    return ordered[lower] + (ordered[upper] - ordered[lower]) * (rank - lower)


def aggregate(runs: list[dict]) -> list[dict]:
    groups: dict[tuple[str, str], list[dict]] = defaultdict(list)
    for run in runs:
        groups[(run["scenario"], run["variant"])].append(run)
    rows: list[dict] = []
    for (scenario, variant), items in sorted(groups.items()):
        row: dict[str, object] = {"scenario": scenario, "variant": variant, "runs": len(items)}
        for field in ("playlist_success_rate", "segment_success_rate", "session_failure_rate", "segment_p95_ms"):
            values = [float(item[field]) for item in items]
            row[f"{field}_mean"] = statistics.fmean(values)
            row[f"{field}_median"] = statistics.median(values)
            row[f"{field}_p95"] = percentile(values, 0.95)
            row[f"{field}_min"] = min(values)
            row[f"{field}_max"] = max(values)
            row[f"{field}_stdev"] = statistics.stdev(values) if len(values) > 1 else 0.0
        rows.append(row)
    return rows


def write_csv(path: Path, rows: list[dict]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if not rows:
        raise SystemExit("No complete raw runs were found.")
    with path.open("w", newline="", encoding="utf-8") as stream:
        writer = csv.DictWriter(stream, fieldnames=list(rows[0]))
        writer.writeheader()
        writer.writerows(rows)


def plot_summary(path: Path, rows: list[dict]) -> None:
    scenarios = sorted({str(row["scenario"]) for row in rows})
    variants = [variant for variant in ("baseline", "adaptive") if any(row["variant"] == variant for row in rows)]
    lookup = {(row["scenario"], row["variant"]): row for row in rows}
    x = list(range(len(scenarios)))
    width = 0.36 if len(variants) == 2 else 0.6
    colors = {"baseline": "#6b7280", "adaptive": "#0f766e"}
    fig, axes = plt.subplots(1, 2, figsize=(12, 4.8), constrained_layout=True)
    for index, variant in enumerate(variants):
        offset = (index - (len(variants) - 1) / 2) * width
        failure = [100 * float(lookup.get((scenario, variant), {}).get("session_failure_rate_mean", math.nan)) for scenario in scenarios]
        failure_err = [100 * float(lookup.get((scenario, variant), {}).get("session_failure_rate_stdev", 0)) for scenario in scenarios]
        latency = [float(lookup.get((scenario, variant), {}).get("segment_p95_ms_mean", math.nan)) for scenario in scenarios]
        latency_err = [float(lookup.get((scenario, variant), {}).get("segment_p95_ms_stdev", 0)) for scenario in scenarios]
        axes[0].bar([value + offset for value in x], failure, width, yerr=failure_err, label=variant, color=colors[variant], capsize=3)
        axes[1].bar([value + offset for value in x], latency, width, yerr=latency_err, label=variant, color=colors[variant], capsize=3)
    axes[0].set_title("HLS session failure rate")
    axes[0].set_ylabel("Failure rate (%)")
    axes[1].set_title("HLS segment P95 latency")
    axes[1].set_ylabel("Milliseconds")
    for axis in axes:
        axis.set_xticks(x, scenarios, rotation=20, ha="right")
        axis.grid(axis="y", alpha=0.25)
        axis.legend(frameon=False)
    fig.suptitle("EdgeRoute Day 6: deterministic baseline vs adaptive routing\nmean ± sample standard deviation; three repetitions required", fontsize=12)
    path.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(path, dpi=180)
    plt.close(fig)


def write_report(path: Path, rows: list[dict]) -> None:
    lines = [
        "# Day 6 processed results",
        "",
        "Generated from immutable files under `../raw/` by `experiments/process_results.py`.",
        "Percentages and latency values are measured results for the recorded profile, not production SLO claims.",
        "",
        "| Scenario | Variant | Runs | Playlist success | Segment success | Session failures | Segment P95 |",
        "|---|---|---:|---:|---:|---:|---:|",
    ]
    for row in rows:
        lines.append(
            f"| {row['scenario']} | {row['variant']} | {row['runs']} | "
            f"{100 * float(row['playlist_success_rate_mean']):.2f}% | "
            f"{100 * float(row['segment_success_rate_mean']):.2f}% | "
            f"{100 * float(row['session_failure_rate_mean']):.2f}% | "
            f"{float(row['segment_p95_ms_mean']):.2f} ms |"
        )
    lines.extend(["", "![Baseline versus adaptive comparison](baseline-vs-adaptive.png)", ""])
    path.write_text("\n".join(lines), encoding="utf-8")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--raw", type=Path, default=Path(__file__).parent / "results" / "raw")
    parser.add_argument("--processed", type=Path, default=Path(__file__).parent / "results" / "processed")
    args = parser.parse_args()
    runs = load_runs(args.raw)
    rows = aggregate(runs)
    write_csv(args.processed / "runs.csv", runs)
    write_csv(args.processed / "summary.csv", rows)
    plot_summary(args.processed / "baseline-vs-adaptive.png", rows)
    write_report(args.processed / "report.md", rows)
    print(f"Processed {len(runs)} runs into {args.processed}")


if __name__ == "__main__":
    main()
