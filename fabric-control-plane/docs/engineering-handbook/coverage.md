# Documentation coverage and publication

[Handbook](README.md) · [Codebase map](02-codebase.md) · [Release evidence](12-evidence.md)

## Coverage

The pinned application trees contain **614 tracked paths across 68 groups**. The file catalogs index all 614 and link the source and recognized declarations at the pinned revision. The API catalog lists **50 route registrations** extracted from the API mux, plus a link to the separate operations listener. The 13 sections cover the primary architecture, workflows, storage, API families, operational procedures, frontend and commercial boundaries.

**No tracked paths are missing from the file inventory.** Narrative coverage focuses on production boundaries and important workflows. It does not individually explain every private helper, generated or vendored file, test assertion, evidence JSON field, or fixture asset. The linked file and symbol catalog is the entry point for those details. The upstream Bevel gitlink is an external source boundary documented in provenance. Its entire upstream source tree is outside the two tracked application trees.

| Open item | Why it remains open | Owner |
|---|---|---|
| Full schema for each API endpoint | Handler and Go/TypeScript sources are authoritative. This handbook summarizes contracts and indexes every registered route. No OpenAPI spec was inferred. | API maintainers |
| Greenfield project selection and live Kubernetes dispatch | Current grant anchors and server-bound selection constrain onboarding. Customer target preview cannot execute. | Identity and platform engineering |
| Live operations, actual payment provider and replacement DR | Controlled tests and prior local evidence cannot establish external behavior. Use the release gate register. | SRE, Finance and Fabric operations |
| Exact hardware and provider limits | The approved environment and independent evidence determine current bounds. | Customer infrastructure and release owners |

## Validation result

The link checker resolved 1,883 handbook references and found all 614 pinned paths in the catalogs. Mermaid 11.12.2 parsed all 12 diagram blocks. The publication destination and its rendering remain unverified.

## Maintenance

When source changes, update the page describing the behavior and its evidence status. Regenerate the catalog against the candidate commit, review link changes and confirm that claims still match the linked tests. Keep prior evidence under its original revision. Use status terms consistently and review customer-facing publication for sensitive data before export.

From the repository root, using the audited source revision:

```powershell
python -B fabric-control-plane/tools/engineering/handbook.py f030569 --generate
$diagramDependencies = Join-Path ([IO.Path]::GetTempPath()) 'sathonate-handbook-mermaid'
npm install --prefix $diagramDependencies --no-audit --no-fund mermaid@11.12.2 jsdom@26.1.0
node fabric-control-plane/tools/engineering/check-handbook-diagrams.mjs fabric-control-plane/docs/engineering-handbook $diagramDependencies
```

Change the documented pinned revision when regenerating for a new release. The handbook folder contains Markdown only. Mermaid diagrams are editable visual source embedded in the Markdown. The check parses Mermaid using the pinned parser; it does not certify Notion rendering.

## Future Notion import

Publication is deferred. Create a private destination page named **Sathonate Fabric SMP — Engineering Handbook**. Use the 13 numbered pages as children, the three catalogs under **Codebase map**, and this page under the handbook root. Use README as the root content. A manual export may ZIP only the Markdown folder and use Notion's ZIP importer. Recreate cross-page links that import as relative Markdown links, and convert Mermaid fences to Mermaid code blocks where needed. Compare each page title, link and diagram to Git after import.

For API publication, connect an authorized Notion integration, grant read/insert/update capabilities on the destination parent page, and provide the parent page URL to the publisher. Keep integration secrets in the authorized connection. Resolve relative links to imported page IDs for handbook cross-links and to pinned Git URLs for repository references. Review source visibility for the intended readers.

[Notion import guide](https://www.notion.com/help/import-data-into-notion) · [Notion connection permissions](https://www.notion.com/en-gb/help/add-and-manage-connections-with-the-api).
