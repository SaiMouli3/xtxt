# XTXT for VS Code

Syntax highlighting, folding and comment toggling for the
[XTXT](https://github.com/SaiMouli3/xtxt) plain-text document format.

- Directives, records and inline formatting highlighted
- `@code(language="…")` blocks highlighted in their own language
- Folding on `@block` / `@endblock`
- `@comment` / `@endcomment` toggling with the usual comment shortcut

## Try the format

<https://saimouli3.github.io/xtxt/>

## Install from source

```sh
git clone https://github.com/SaiMouli3/xtxt
ln -s "$PWD/xtxt/editors/vscode" ~/.vscode/extensions/xtxt
```

Reload VS Code and open any `.xtxt` file.
