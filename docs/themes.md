# Themes

## Setting a theme

Set the theme in your `tiki-config.yaml` under the `appearance` section (see [Configuration](config.md) for file locations and which files tiki reads):

```yaml
appearance:
  theme: dracula
```

Available values:
- `auto` — detects your terminal background and picks `dark` or `light` automatically (default)
- `dark`, `light` — built-in base themes
- Any named theme listed below

## A theme per project

A project's own `tiki-config.yaml` merges over your global one, so a project can pick its own theme
without repeating your other settings. Put just this in the project folder's `tiki-config.yaml`:

```yaml
appearance:
  theme: nord
```

A theme recolours everything. To tell projects apart with a lighter touch, give each one a
`project.color` instead (see [Configuration](config.md#tiki-configyaml)): it colours the logo, the
project name in the header and the directory block in the bottom bar, and leaves the theme alone.

## Dark themes

### dark

The built-in dark base theme. Used by `auto` when a dark terminal background is detected.

![dark](themes/dark.png)

### dracula

![dracula](themes/dracula.png)

### tokyo-night

![tokyo-night](themes/tokyo-night.png)

### gruvbox-dark

![gruvbox-dark](themes/gruvbox-dark.png)

### catppuccin-mocha

![catppuccin-mocha](themes/catppuccin-mocha.png)

### solarized-dark

![solarized-dark](themes/solarized-dark.png)

### nord

![nord](themes/nord.png)

### monokai

![monokai](themes/monokai.png)

### one-dark

![one-dark](themes/one-dark.png)

## Light themes

### light

The built-in light base theme. Used by `auto` when a light terminal background is detected.

![light](themes/light.png)

### catppuccin-latte

![catppuccin-latte](themes/catppuccin-latte.png)

### solarized-light

![solarized-light](themes/solarized-light.png)

### gruvbox-light

![gruvbox-light](themes/gruvbox-light.png)

### github-light

![github-light](themes/github-light.png)
