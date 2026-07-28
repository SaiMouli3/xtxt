"""The Python SDK is checked against the same fixtures as the Go reference."""
import json
import pathlib

import pytest

import xtxt

CASES = sorted((pathlib.Path(__file__).parents[2] / "conformance" / "cases").glob("*.xtxt"))
assert CASES, "conformance cases not found"


@pytest.mark.parametrize("case", CASES, ids=lambda p: p.stem)
def test_matches_reference(case):
    expected = json.loads(case.with_suffix(".json").read_text())
    res = xtxt.parse(case.read_text())

    assert xtxt.canonical(res.doc) == expected["ast"]
    assert xtxt.canonical_issues(list(res.issues) + xtxt.validate(res.doc)) == expected["issues"]


def test_inline():
    assert xtxt.inline_html("**b** and `a<b`") == "<strong>b</strong> and <code>a&lt;b</code>"
    assert xtxt.inline_text("**b** and [x](y)") == "b and x"


def test_extract():
    doc = xtxt.parse(
        "# T\n\n- [x] done\n\n@task\nTitle: Ship it\nStatus: Done\n@endtask\n"
    ).doc
    got = xtxt.extract(doc)
    assert [t["title"] for t in got["tasks"]] == ["done", "Ship it"]
    assert all(t["done"] for t in got["tasks"])
    assert got["outline"] == [{"level": 1, "text": "T", "line": 1}]
