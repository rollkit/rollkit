# Rollkit Specifications

## Building From Source

Install [mdbook](https://rust-lang.github.io/mdBook/guide/installation.html) and [mdbook-toc](https://github.com/badboy/mdbook-toc):

```sh
cargo install mdbook
cargo install mdbook-toc
```

To build book:

```sh
mdbook build
```

To serve locally:

```sh
mdbook serve
```

## Contributing

Specifications are written and stored in the same directory with the implementation code. Since `mdbook` does not support referencing files outside of the book directory, we use a symlink to the `src/specs` directory. This allows us to reference the specifications from the `README.md` and `SUMMARY.md` files in the `src` directory.

To create a symlink:

```sh
cd src/specs
ln -s path/to/spec.md
```

Markdown files must conform to [GitHub Flavored Markdown](https://github.github.com/gfm/). Markdown must be formatted with:

- [markdownlint](https://github.com/DavidAnson/markdownlint)
- [Markdown Table Prettifier](https://github.com/darkriszty/MarkdownTablePrettify-VSCodeExt)
