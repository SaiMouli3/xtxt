//! The Rust SDK is checked against the same fixtures as the Go reference.
//!
//! `canonical` lives here rather than in the crate so that the library itself
//! stays dependency-free: only the test needs to speak JSON.

use serde_json::{json, Value};
use std::fs;
use std::path::{Path, PathBuf};

fn cases_dir() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../../conformance/cases")
        .canonicalize()
        .expect("conformance cases directory")
}

/// The normalised shape every implementation must produce. Every field is
/// present with an explicit default, so an implementation cannot pass by
/// omitting something the reference emits.
fn canonical(doc: &xtxt::Document) -> Value {
    let nodes: Vec<Value> = doc
        .nodes
        .iter()
        .map(|n| {
            let args: Vec<Value> = n
                .args
                .iter()
                .map(|a| json!({ "key": a.key, "value": a.value }))
                .collect();
            let items: Vec<Value> = n
                .items
                .iter()
                .map(|it| {
                    json!({
                        "text": it.text,
                        "ordered": it.ordered,
                        "checked": match it.checked {
                            Some(v) => Value::Bool(v),
                            None => Value::Null,
                        },
                    })
                })
                .collect();
            json!({
                "kind": n.kind.as_str(),
                "name": n.name,
                "level": n.level,
                "text": n.text,
                "args": args,
                "items": items,
                "line": n.line,
            })
        })
        .collect();
    json!({ "version": doc.version, "nodes": nodes })
}

fn canonical_issues(issues: &[xtxt::Issue]) -> Value {
    Value::Array(
        xtxt::sort_issues(issues)
            .iter()
            .map(|i| json!({ "severity": i.severity.as_str(), "line": i.line }))
            .collect(),
    )
}

#[test]
fn matches_the_reference_implementation() {
    let dir = cases_dir();
    let mut inputs: Vec<PathBuf> = fs::read_dir(&dir)
        .expect("read cases")
        .filter_map(|e| e.ok().map(|e| e.path()))
        .filter(|p| p.extension().map(|x| x == "xtxt").unwrap_or(false))
        .collect();
    inputs.sort();
    assert!(!inputs.is_empty(), "no conformance cases found in {dir:?}");

    let mut failures = Vec::new();
    for input in &inputs {
        let src = fs::read_to_string(input).expect("read case");
        let expected: Value = serde_json::from_str(
            &fs::read_to_string(input.with_extension("json")).expect("read expectation"),
        )
        .expect("parse expectation");

        let res = xtxt::parse(&src);
        let mut issues = res.issues.clone();
        issues.extend(xtxt::validate(&res.doc));

        let name = input.file_name().unwrap().to_string_lossy().to_string();
        if canonical(&res.doc) != expected["ast"] {
            failures.push(format!(
                "{name}: AST mismatch\n  got:  {}\n  want: {}",
                canonical(&res.doc),
                expected["ast"]
            ));
        }
        if canonical_issues(&issues) != expected["issues"] {
            failures.push(format!(
                "{name}: issues mismatch\n  got:  {}\n  want: {}",
                canonical_issues(&issues),
                expected["issues"]
            ));
        }
    }
    assert!(
        failures.is_empty(),
        "{} of {} cases failed:\n{}",
        failures.len(),
        inputs.len(),
        failures.join("\n")
    );
}
