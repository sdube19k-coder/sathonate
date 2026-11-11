"""Render and inspect both Sprint 8 customer profiles without a cluster."""
import argparse
from pathlib import Path
import subprocess

import yaml


def check(helm):
    root = Path(__file__).resolve().parents[2]
    chart = root / "deploy/charts/sathonate"
    ingress = ["-f", str(chart / "profiles/customer-ingress.yaml")]
    gateway = ingress + ["-f", str(chart / "profiles/customer-gateway.yaml")]

    def run(*args, passed=True):
        result = subprocess.run([helm, *map(str, args)], capture_output=True, text=True)
        if (result.returncode == 0) != passed:
            raise AssertionError(result.stderr or result.stdout)
        return result.stdout

    for values, is_ingress in ((ingress, True), (gateway, False)):
        run("lint", chart, "--strict", *values)
        documents = list(filter(None, yaml.safe_load_all(
            run("template", "fixture", chart, "--namespace", "fixture", *values))))
        deployments = {doc["metadata"]["labels"]["app.kubernetes.io/component"]: doc
                       for doc in documents if doc["kind"] == "Deployment"}
        assert set(deployments) == {"api", "console"}
        assert not any(doc["kind"] in ("Secret", "RoleBinding", "PersistentVolumeClaim") for doc in documents)
        for component, deployment in deployments.items():
            pod = deployment["spec"]["template"]["spec"]
            assert pod["automountServiceAccountToken"] is False
            assert deployment["spec"]["selector"]["matchLabels"] == deployment["spec"]["template"]["metadata"]["labels"]
            container = pod["containers"][0]
            assert container["securityContext"]["readOnlyRootFilesystem"]
            assert "@sha256:" in container["image"]
            if component == "api":
                assert container["args"][container["args"].index("-deployment-mode") + 1] == "disabled"
                dsn = next(env for env in container["env"] if env["name"] == "DATABASE_URL")
                assert set(dsn) == {"name", "valueFrom"} and "secretKeyRef" in dsn["valueFrom"]
        edges = [doc for doc in documents if doc["kind"] == "Ingress"]
        rules = [doc for doc in documents if doc["kind"] == "PrometheusRule"]
        scrapers = [doc for doc in documents if doc["kind"] == "ServiceMonitor"]
        assert len(edges) == len(rules) == len(scrapers) == int(is_ingress)
        if is_ingress:
            spec = edges[0]["spec"]
            assert spec["tls"][0]["hosts"] == [spec["rules"][0]["host"]]
            paths = spec["rules"][0]["http"]["paths"]
            assert [(entry["path"], entry["backend"]["service"]["name"]) for entry in paths] == [
                ("/api", "fixture-sathonate-api"), ("/", "fixture-sathonate-console")]
            assert len(rules[0]["spec"]["groups"][0]["rules"]) == 3
        for doc in documents:
            if doc["kind"] == "NetworkPolicy":
                source = doc["spec"]["ingress"][0]["from"][0]
                assert source["namespaceSelector"]["matchLabels"] and source["podSelector"]["matchLabels"]
        print(f"{'Ingress' if is_ingress else 'Gateway'} profile lint and render PASS ({len(documents)} resources)")

    for key in ("apiImage=registry.example.test/api:latest", "ingress.tlsSecret=",
                "postgres.existingSecret=", "traffic.namespace=", "monitoring.namespace="):
        run("template", "fixture", chart, *ingress, "--set-string", key, passed=False)
    print("Unsafe/missing configuration rejected (5 cases)")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--helm", default="helm")
    check(parser.parse_args().helm)
