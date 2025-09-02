# Static CSS Files

This directory contains locally embedded CSS files to avoid external CDN dependencies.

## Files

### bootstrap.min.css
- **Source**: Bootstrap 4.6.2
- **URL**: https://cdn.jsdelivr.net/npm/bootstrap@4.6.2/dist/css/bootstrap.min.css
- **License**: MIT License
- **Purpose**: Provides responsive grid system, form styling, and UI components

### fontawesome.min.css  
- **Source**: Font Awesome 5.15.4
- **URL**: https://cdnjs.cloudflare.com/ajax/libs/font-awesome/5.15.4/css/all.min.css
- **License**: Font Awesome Free License
- **Purpose**: Provides icon fonts used in the navigation and UI

## Font Files

The following font files are located in `../webfonts/` and are required by Font Awesome:

### fa-solid-900.woff2
- **Source**: Font Awesome 5.15.4 Solid Icons
- **URL**: https://cdnjs.cloudflare.com/ajax/libs/font-awesome/5.15.4/webfonts/fa-solid-900.woff2
- **Purpose**: Solid style icons (used in navigation)

### fa-regular-400.woff2
- **Source**: Font Awesome 5.15.4 Regular Icons
- **URL**: https://cdnjs.cloudflare.com/ajax/libs/font-awesome/5.15.4/webfonts/fa-regular-400.woff2
- **Purpose**: Regular/outline style icons

### fa-brands-400.woff2
- **Source**: Font Awesome 5.15.4 Brand Icons
- **URL**: https://cdnjs.cloudflare.com/ajax/libs/font-awesome/5.15.4/webfonts/fa-brands-400.woff2
- **Purpose**: Brand and logo icons

## Updating Files

To update these files to newer versions:

1. Download the latest Bootstrap 4.x CSS:
   ```bash
   curl -o bootstrap.min.css https://cdn.jsdelivr.net/npm/bootstrap@4/dist/css/bootstrap.min.css
   ```

2. Download the latest Font Awesome CSS:
   ```bash
   curl -o fontawesome.min.css https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.0.0/css/all.min.css
   ```

3. Download the latest Font Awesome font files:
   ```bash
   cd ../webfonts
   curl -o fa-solid-900.woff2 https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.0.0/webfonts/fa-solid-900.woff2
   curl -o fa-regular-400.woff2 https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.0.0/webfonts/fa-regular-400.woff2
   curl -o fa-brands-400.woff2 https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.0.0/webfonts/fa-brands-400.woff2
   ```

## Integration

These files are embedded into the Go binary using `//go:embed static/*` and served via HTTP at `/mcsdadmin/static/css/` endpoints.

The base template references these files as:
- `/mcsdadmin/static/css/bootstrap.min.css`
- `/mcsdadmin/static/css/fontawesome.min.css`

Font files are automatically referenced by the Font Awesome CSS and served from `/mcsdadmin/static/webfonts/`.
