"""Refresh source catalogs from a pinned Git revision and check handbook links."""
import argparse
from collections import defaultdict
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[3]
HERE = ROOT / "fabric-control-plane/docs/engineering-handbook"
REMOTE = "https://github.com/sdube19k-coder/sathonate"

def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT).decode("utf-8")

def link(revision, path, line=None, label=None):
    suffix = f"#L{line}" if line else ""
    return f"[{label or path.split('/')[-1]}]({REMOTE}/blob/{revision}/{path}{suffix})"

def generate(revision):
    paths = git("ls-tree", "-r", "--name-only", revision, "fabric-control-plane", "fabric-console").splitlines()
    groups = defaultdict(list)
    api = []
    for path in paths:
        pieces = path.split("/")
        section = pieces[0] + "/" + pieces[1]
        if pieces[0] == "fabric-control-plane" and pieces[1] in ("internal", "cmd", "tools", "deploy") and len(pieces) > 3:
            section += "/" + pieces[2]
        if len(pieces) == 2:
            section = pieces[0] + " (root)"
        groups[section].append(path)

    for filename, prefix, title in (
        ("catalog-backend.md", "fabric-control-plane", "Backend file and symbol catalog"),
        ("catalog-console.md", "fabric-console", "Console file and symbol catalog"),
    ):
        lines = [f"# {title}", "", "[Handbook](README.md) · [Module map](02-codebase.md)", "",
                 f"Git tree at `{revision}`. The index shows representative declarations, test names and direct imports. "
                 "See the linked source for full control flow. Examples, evidence and assets are indexed by path only.", ""]
        for section, entries in sorted(groups.items()):
            if not section.startswith(prefix):
                continue
            lines += [f"## {section}", ""]
            for path in entries:
                entry = f"- {link(revision, path, label=path.split('/')[-1])}"
                if path.endswith((".go", ".py", ".ts", ".tsx", ".mjs", ".ps1")) and "/docs/" not in path:
                    raw = git("show", f"{revision}:{path}")
                    symbols, imports = [], []
                    patterns = (
                        r"^func (?:\(([^)]*)\) )?([A-Za-z_]\w*)\(",
                        r"^type (\w+) (?:interface|struct)\b",
                        r"^(?:export )?(?:default )?(?:async )?function (\w+)",
                        r"^export (?:type|interface|const|class) (\w+)",
                        r"^(?:async )?def (\w+)\(",
                        r"^class (\w+)",
                    )
                    for n, row in enumerate(raw.splitlines(), 1):
                        for pattern in patterns:
                            match = re.match(pattern, row)
                            if match:
                                symbol = match.groups()[-1]
                                if symbol != "main" or not symbols:
                                    symbols.append(link(revision, path, n, symbol))
                                break
                        match = re.search(r'"sathonate\.local/fabric-control-plane/internal/([\w-]+)"|from ["\'](\./[\w.-]+)["\']', row)
                        if match:
                            imports.append(next(v for v in match.groups() if v))
                        if path.endswith("/httpapi/api.go"):
                            route = re.search(r'mux\.HandleFunc\("([A-Z]+ /[^"]+)"(?:,\s*([\w.]+))?', row)
                            if route:
                                method, handler = route.groups()
                                api.append(f"| `{method}` | {link(revision,path,n,handler or 'inline handler')} |")
                            if "identity.CustomerInvitationAcceptance" in row and "HandleFunc" in row:
                                api.append(f"| `POST /api/v1/customer/invitations/accept` | {link(revision,path,n,'customerAction')} |")
                    if symbols:
                        entry += " · " + ", ".join(symbols[:6])
                    if imports:
                        entry += " · Imports " + ", ".join(f"`{x}`" for x in sorted(set(imports)))
                lines.append(entry)
            lines.append("")
        (HERE / filename).write_text("\n".join(lines), encoding="utf-8", newline="\n")
    (HERE / "catalog-api.md").write_text("\n".join([
        "# API route catalog", "", "[Handbook](README.md) · [API contracts](05-api.md)", "",
        f"Route registration at `{revision}`. Wrappers may add authentication, entitlements and runner gates. "
        "The operations listener is defined separately.", "", "| Route | Implementation |", "|---|---|",
        *api, "", "[Separate operations listener](../../internal/telemetry/http.go)", "",
    ]), encoding="utf-8", newline="\n")
    print(f"Indexed {len(paths)} tracked paths in {len(groups)} groups, {len(api)} public API route registrations.")

def validate(revision):
    pinned = set(git("ls-tree", "-r", "--name-only", revision).splitlines())
    issues = []
    indexed_paths = set()
    link_count = diagram_count = 0
    for page in HERE.glob("*.md"):
        content = page.read_text(encoding="utf-8-sig")
        diagram_count += len(re.findall(r"^\x60{3}mermaid\s*$", content, re.M))
        for destination in re.findall(r"\[[^\]\n]+\]\(([^)\s]+)\)", content):
            link_count += 1
            if destination.startswith(REMOTE + "/blob/"):
                pin, path = destination.split("/blob/", 1)[1].split("/", 1)
                path, _, anchor = path.partition("#")
                if page.name in ("catalog-backend.md", "catalog-console.md") and not anchor:
                    indexed_paths.add(path)
                if pin != revision or path not in pinned:
                    issues.append(f"{page.name}: invalid pinned reference {destination}")
                elif anchor:
                    # Line links are generated from declarations read at the same pinned revision.
                    if not re.fullmatch(r"L[1-9][0-9]*", anchor):
                        issues.append(f"{page.name}: invalid line anchor {destination}")
            elif not destination.startswith(("https://", "http://")):
                target = (page.parent / destination.split("#", 1)[0]).resolve()
                if not target.exists() or not target.is_relative_to(ROOT):
                    issues.append(f"{page.name}: missing or foreign relative path {destination}")
    expected = {path for path in pinned if path.startswith(("fabric-control-plane/", "fabric-console/"))}
    for missing in sorted(expected - indexed_paths):
        issues.append(f"catalog missing tracked path: {missing}")
    if issues:
        raise SystemExit("\n".join(issues))
    print(f"Validated {link_count} handbook links; found {diagram_count} Mermaid blocks and {len(indexed_paths)} indexed paths.")

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("revision")
    parser.add_argument("--generate", action="store_true")
    args = parser.parse_args()
    rev = git("rev-parse", "--verify", args.revision + "^{commit}").strip()
    if args.generate:
        generate(rev)
    validate(rev)
