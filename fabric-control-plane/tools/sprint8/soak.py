"""Read-only HTTP soak for the isolated local API operations listener."""
import argparse
import concurrent.futures
import http.client
import json
import math
import time


def probe(port):
    start = time.monotonic()
    conn = http.client.HTTPConnection("127.0.0.1", port, timeout=2)
    try:
        conn.request("GET", "/readyz")
        with conn.getresponse() as response:
            ok = response.status == 204
    except (OSError, http.client.HTTPException):
        ok = False
    finally:
        conn.close()
    return ok, time.monotonic() - start


def run(port, seconds, rate, workers):
    if not (1 <= port <= 65535 and 1 <= seconds <= 86400 and
            1 <= rate <= 100 and 1 <= workers <= 64):
        raise ValueError("invalid port, duration, rate or concurrency")
    planned = seconds * rate
    if planned < 20:
        raise ValueError("at least 20 probes required")
    complete = successes = within_slo = dropped = 0
    active = set()
    start = time.monotonic()

    def collect(done):
        nonlocal complete, successes, within_slo
        for future in done:
            ok, elapsed = future.result()
            complete += 1
            successes += ok
            within_slo += ok and elapsed <= 0.5

    with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
        for index in range(planned):
            delay = start + index / rate - time.monotonic()
            if delay > 0:
                time.sleep(delay)
            done = {future for future in active if future.done()}
            collect(done)
            active -= done
            if len(active) >= workers or delay < -1 / rate:
                dropped += 1
            else:
                active.add(pool.submit(probe, port))
        collect(concurrent.futures.as_completed(active))
    passed = successes >= math.ceil(planned * 0.99) and within_slo >= math.ceil(planned * 0.95)
    return {
        "status": "PASS" if passed else "FAIL",
        "configuredSeconds": seconds,
        "elapsedSeconds": round(time.monotonic() - start, 3),
        "targetRequestsPerSecond": rate,
        "concurrency": workers,
        "planned": planned,
        "completed": complete,
        "dropped": dropped,
        "successful": successes,
        "successfulWithin500ms": within_slo,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=9090)
    parser.add_argument("--seconds", type=int, required=True)
    parser.add_argument("--rate", type=int, default=5)
    parser.add_argument("--workers", type=int, default=8)
    args = parser.parse_args()
    try:
        result = run(args.port, args.seconds, args.rate, args.workers)
    except ValueError as exc:
        parser.error(str(exc))
    print(json.dumps(result, sort_keys=True))
    return result["status"] != "PASS"


if __name__ == "__main__":
    raise SystemExit(main())
