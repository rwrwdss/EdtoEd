"""Exercise the platform integration flow with synthetic learner snapshots."""

import argparse
import json
import os
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen
from uuid import uuid4


def analyze(base_url, token, snapshot, *, allow_incomplete=False):
    request = Request(
        f"{base_url.rstrip('/')}/v1/analyze",
        data=json.dumps(snapshot).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    request.add_unredirected_header("Authorization", f"Bearer {token}")
    try:
        with urlopen(request, timeout=10) as response:
            return json.load(response)
    except HTTPError as error:
        with error:
            if allow_incomplete and error.code == 409:
                return None
            message = error.read().decode("utf-8", errors="replace").strip()
            raise RuntimeError(f"analysis returned HTTP {error.code}: {message}") from error


def pilot(base_url, token):
    cohort_id = f"demo-org:course-1:{uuid4().hex}"
    snapshots = [
        {
            "student_id": student_id,
            "topic_id": "module-1",
            "cohort_id": cohort_id,
            "window_start": 1704067200,
            "window_end": 1706572800,
            "clicks_count": clicks,
            "test_score": score,
            "deadline_delay": delay,
        }
        for student_id, clicks, score, delay in [
            ("learner-1", 0, 0.0, 0.0),
            ("learner-2", 12, 0.6, 2.0),
            ("learner-3", 24, 0.9, -1.0),
        ]
    ]

    # A 409 can follow a successful write while the cohort is incomplete.
    # Ignore provisional scores until every learner has been submitted.
    for snapshot in snapshots:
        analyze(base_url, token, snapshot, allow_incomplete=True)

    scores = []
    for snapshot in snapshots:
        result = analyze(base_url, token, snapshot)
        scores.append(
            {
                "student_id": result["student_id"],
                "academic_risk": result["burnout_risk"],
                "requires_action": result["requires_action"],
                "action": result["action_type"],
            }
        )
    return {"cohort_id": cohort_id, "scores": scores}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://localhost:8080", help="EdtoEd base URL")
    args = parser.parse_args()
    token = os.environ.get("EDTOED_API_KEY")
    if not token:
        parser.error("set EDTOED_API_KEY to the platform bearer key")
    try:
        print(json.dumps(pilot(args.url, token), indent=2))
    except (URLError, RuntimeError, TimeoutError) as error:
        parser.exit(1, f"Pilot failed: {error}\n")


if __name__ == "__main__":
    main()
