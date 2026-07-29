# Launch material

Drafts to edit, not to paste verbatim — the voice should be yours. Everything
here assumes the packages are published and the demo is live, because a launch
that lands on an uninstallable repo converts nobody.

---

## Pre-flight

- [ ] `pip install xtxt` works
- [ ] `npm install xtxt-js` works
- [ ] `go install github.com/SaiMouli3/xtxt/cmd/xtxt@latest` works
- [ ] `cargo add xtxt` works
- [ ] <https://saimouli3.github.io/xtxt/> loads and the demo renders
- [ ] README badges are all green, no 404s
- [ ] VS Code extension published (or the README says "install from source")
- [ ] You have a free 6–8 hours to answer comments

That last one is not optional. On Hacker News, the author being absent is the
difference between a front page and a dead post.

---

## Show HN

**Title** (80 char limit — this one is 79):

```
Show HN: XTXT – a plain-text document format agents can parse without guessing
```

Alternatives if that reads as too AI-forward:

```
Show HN: XTXT – plain text with images, tables and typed records
Show HN: A document format that is readable in cat and parseable by an agent
```

**Body:**

> I kept hitting the same wall taking notes: `.txt` can't hold an image,
> `.docx` can't be diffed, and Markdown makes anything structured — a task, a
> decision, a spec field — into prose that something downstream has to guess
> at.
>
> XTXT is my attempt at the middle. One syntax for everything non-textual:
> `@name(args)` for a single item, `@name … @endname` for a block.
>
> ```
> @task
> Title: Ship the reference parser
> Status: In Progress
> Owner: Subbu
> @endtask
> ```
>
> That block round-trips through the parser and comes out of `xtxt extract` as
> JSON, so an agent gets the structure instead of inferring it. The file is
> still plain UTF-8 you can open in vi and diff in git.
>
> The design decision I care most about: **an unknown directive is a warning,
> never an error.** A reader built today opens a document written against a
> later version of the spec, renders every paragraph, and shows a placeholder
> where the new thing was. That is what makes it safe to extend.
>
> There is a spec, a Go reference implementation, ports in Python, JavaScript,
> Rust and Java, and a conformance suite of fixtures all five are held to — a
> sixth implementation joins by passing that directory. It is MIT.
>
> Live demo (runs entirely in your browser): https://saimouli3.github.io/xtxt/
> Repo: https://github.com/SaiMouli3/xtxt
> Spec: https://github.com/SaiMouli3/xtxt/blob/main/SPEC.md
>
> I know exactly which xkcd this invites. I'd genuinely like to hear where the
> design is wrong before more gets built on it.

**Timing:** Tuesday–Thursday, 9–10am ET. Avoid Fridays and weekends.

---

## Answers to the questions you will definitely get

Have these ready. Short, non-defensive, and concede the real points.

**"Why not Markdown with YAML front matter?"**
> Front matter is one record, at the top of the file. XTXT lets a record sit
> anywhere — next to the thing it describes — and names it. A doc with twelve
> decisions in it can't express that as front matter. Also, front matter is a
> convention Markdown parsers bolt on, not something the format defines, so
> two tools disagree about it constantly.

**"Why not just use MDX / Org-mode / reStructuredText / AsciiDoc?"**
> Org-mode is the closest and genuinely good — it has drawers and properties
> that do a lot of this. The honest answer is that Org's grammar is large and
> Emacs-shaped, and there is no small spec you can implement in an afternoon
> in another language. AsciiDoc is closer still in spirit but heavier. I
> wanted something a single file of Go, Python or JS could parse completely.

**"xkcd 927."**
> Fair. My defence is that I wrote the spec and the conformance suite before
> the promotion, so if it dies it at least dies as a document anyone can
> implement rather than one tool's private syntax.

**"How is this different from JSON/YAML with a text field?"**
> Inverted priority. Those are data files with prose stuffed in. This is a
> prose file with data in it, and it stays readable when the tooling is gone.

**"Why should an LLM care? They read Markdown fine."**
> They read it fine and they infer structure inconsistently. If a doc has 40
> tasks in prose, extraction is a model call with a failure rate. Here it is a
> parse. The point is not that models can't cope — it's that they shouldn't
> have to for something the author already knew.

**"Does it handle nesting / lists in tables / footnotes in code blocks?"**
> No nesting in 1.0, deliberately — it is what keeps a parser one pass and an
> implementation an afternoon. If that's disqualifying for your use, say so;
> it is the constraint I'm least sure about.

---

## Reddit

Post to **r/ObsidianMD** and **r/PKMS** first — note-takers feel "my images
are external files" every day, and they adopt formats fast. r/programming
last; it's high volume, low conversion, and hostile to new formats.

**r/ObsidianMD angle** (do *not* reuse the HN post — lead with their pain):

> **Notes that keep images inside the file, and still diff in git**
>
> I got tired of my vault being a folder of `.md` plus a folder of loose PNGs
> where half the links eventually break. So I wrote a format where the image,
> the table, the chart and the task metadata all live in the one text file.
>
> [demo gif or screenshot]
>
> It's plain UTF-8 — opens in any editor, diffs cleanly, no lock-in. There's a
> browser demo if you want to poke at it: [link]
>
> Not trying to replace Markdown in anyone's vault. Curious whether the
> "records anywhere in the doc" idea is useful to people who use Dataview.

Attach a **screenshot or 20-second screen recording**. Reddit posts with a
visual do several times better than text-only.

---

## X / Bluesky

Thread, one idea per post, visual on the first:

1. `.txt` can't hold an image. `.docx` can't be diffed. Markdown makes
   structure into prose something has to guess at. So I built XTXT. [demo gif]
2. One syntax for everything: `@name(args)` for a thing, `@name … @endname`
   for a block. That's the format. [code screenshot]
3. Records sit next to the prose they describe — and `xtxt extract` hands them
   to an agent as JSON instead of making it infer them. [side-by-side]
4. Unknown directives are a warning, never an error. A reader from today stays
   useful on a document written tomorrow.
5. Spec, Go reference implementation, ports in Python, JS, Rust and Java, one
   conformance suite all five pass. MIT. [link]

---

## Longer term (this is what actually creates adoption)

Announcements spike and decay. Pull comes from integrations:

- **Obsidian plugin** — highest value. That community maps directly onto
  records, and it's the audience that already wanted this.
- **MCP server** wrapping `xtxt extract` — makes every agent an XTXT reader
  with no per-agent work.
- **linguist / Pygments lexer** — GitHub colouring `.xtxt` files is free
  credibility on every repo that contains one.
- **Pandoc writer** — puts XTXT in the conversion graph everyone already uses.

Pick one and ship it a few weeks after launch, when the first wave has
decayed. A second, smaller wave from "XTXT now works in X" compounds better
than a bigger first one.

---

## What success looks like

Be realistic so you don't read a normal outcome as failure. For a solo format
with a spec and five implementations:

- **Good day one:** 200–800 GitHub stars, front page of HN for a few hours,
  30–50 comments, a handful of genuinely useful design critiques.
- **Good month one:** a few hundred more stars, 2–5 outside contributors,
  one person who actually uses it for real work and files real issues.
- **The signal that matters:** somebody writes a fourth implementation, or
  files a spec bug rather than a code bug. That is when it stops being your
  project and starts being a format.

Stars are vanity. The conformance suite passing in a language you didn't write
is the metric.
