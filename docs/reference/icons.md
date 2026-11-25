# Icons Reference

Plux pages show icons from two built-in sets and from custom SVG icon sets (`THM-005`). The design is [ADR-0032 § Icons](../adr/0032-design-tokens-to-material-and-cupertino.md#icons-thm-005).

## 1. Naming an icon

An icon is an `IconData` value: a `name` and a `set`, `material` (the default) or `cupertino`.

```json
{ "type": "Icon", "props": { "icon": { "name": "arrow_back" }, "fill": 1, "weight": 600 } }
```

| Set | Font | Names |
|---|---|---|
| `material` | Material Symbols Outlined, the variable font (axes `FILL`, `wght`, `GRAD`, `opsz`) | the names of [Material Symbols](https://fonts.google.com/icons?icon.set=Material+Symbols), e.g. `home`, `arrow_back` |
| `cupertino` | the font of the `cupertino_icons` package | the member names of Flutter's `CupertinoIcons`, e.g. `left_chevron`, `share` |

The names are checked at compile time: a name its set does not have is `PLX-1123`. An icon is written literally, or chosen by an expression from the icons its plugin declares, because the server delivers only the glyphs a bundle names (`PLX-1123` otherwise).

An icon read from data, such as a category's, is chosen by an expression: `{"$expr": "{\"name\": item.icon}"}`, or a `name` bound to one. The plugin lists the icons its expressions may choose in `dynamicIcons` (`[{"name": "school"}, {"name": "left_chevron", "set": "cupertino"}]`, material when `set` is absent); a plugin that binds an icon to an expression and declares none is `PLX-1123`, and so is a declared name its set lacks. Their glyphs join the plugin's icon font once an expression uses them, so the font grows only by what is declared. A name the expression gives at run time that the list lacks has no glyph: the runtime reports it and draws no icon.

Icons Flutter mirrors in right-to-left text (`arrow_back`, `left_chevron` and the other directional icons) are mirrored in Plux pages too. The `Icon` widget's `fill`, `weight`, `grade` and `opticalSize` set the Material Symbols axes; the Cupertino font has none.

## 2. Delivery

When a bundle is published, the server subsets each set's font to the icons the bundle's documents use and indexes the result as the bundle's asset `@icons/<set>` (`CMP-032`). A device downloads it with the bundle's other assets, checks it against its hash, and loads it the first time a page needs it. A plugin that uses two Material icons carries about 45 KB of icon font; the full Material Symbols font is 10.7 MB.

The fonts are pinned in `backend/internal/icons/fonts` (`icons.lock`); `import.sh` moves a pin, and `go generate ./internal/icons` regenerates the name tables.

## 3. Custom icon sets

A custom icon is an SVG asset shown with the `Image` widget; the server compiles it to `vector_graphics` (`CMP-031`). The `Image`'s `color` tints it, blended as `colorBlendMode` says (`srcIn` by default, as Flutter's `Image`), so a one-colour icon takes the colour of its place.

Studio's **Icons** page lists the app's SVG assets by folder and uploads new ones under `icons/`. The canvas draws an SVG from its stored file: the browser draws it to a PNG at four times its declared size (the longer side 256 to 1024 pixels), with no script and nothing fetched, and CanvasKit draws that picture with the tint. The API lists the stored file as the asset's `original` form, beside the `vector` form devices read.
