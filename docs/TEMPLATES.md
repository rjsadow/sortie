# Template Marketplace

The Template Marketplace provides pre-configured application templates that
administrators can easily add to their Launchpad instance.

## Overview

Templates are pre-defined container application configurations that include:

- Container image and recommended resource limits
- Application metadata (name, description, icon)
- Tags for easy discovery
- Documentation links

## Using the Template Browser

1. Click the **Templates** button in the header
2. Browse templates by category using the sidebar
3. Search for specific templates by name, description, or tags
4. Click a template to view details
5. Customize the Application ID (must be unique)
6. Click **Add to Launchpad** to add the application

## Available Categories

| Category | Description |
|----------|-------------|
| Development | IDEs, code editors, CI/CD tools |
| Productivity | Office suites, file sync, collaboration |
| Communication | Team chat and messaging platforms |
| Browsers | Containerized web browsers |
| Monitoring | Dashboards, metrics, and alerting |
| Databases | Database administration tools |
| Creative | Image editing and design tools |

## Template Catalog

### Development

| Template | Description | Image |
|----------|-------------|-------|
| VS Code Server | Browser-based Visual Studio Code | `lscr.io/linuxserver/code-server` |
| GitLab CE | Self-hosted Git and CI/CD platform | `gitlab/gitlab-ce` |
| Jenkins | Automation server for CI/CD | `jenkins/jenkins:lts` |
| Gitea | Lightweight self-hosted Git service | `gitea/gitea` |
| JupyterLab | Interactive notebooks and code | `jupyter/datascience-notebook` |

### Productivity

| Template | Description | Image |
|----------|-------------|-------|
| LibreOffice | Full-featured office suite | `lscr.io/linuxserver/libreoffice` |
| Nextcloud | File sync and collaboration | `nextcloud` |
| OnlyOffice | Online document editing | `onlyoffice/documentserver` |

### Communication

| Template | Description | Image |
|----------|-------------|-------|
| Mattermost | Team messaging platform | `mattermost/mattermost-team-edition` |
| Rocket.Chat | Team communication with chat and video | `rocket.chat` |

### Browsers

| Template | Description | Image |
|----------|-------------|-------|
| Firefox | Privacy-focused browser | `lscr.io/linuxserver/firefox` |
| Chromium | Open-source browser | `lscr.io/linuxserver/chromium` |

### Monitoring

| Template | Description | Image |
|----------|-------------|-------|
| Grafana | Analytics and visualization | `grafana/grafana` |
| Prometheus | Monitoring and alerting toolkit | `prom/prometheus` |
| Uptime Kuma | Website and service monitoring | `louislam/uptime-kuma` |

### Databases

| Template | Description | Image |
|----------|-------------|-------|
| pgAdmin | PostgreSQL administration | `dpage/pgadmin4` |
| Adminer | Multi-database management | `adminer` |

### Creative

| Template | Description | Image |
|----------|-------------|-------|
| GIMP | Image manipulation and editing | `lscr.io/linuxserver/gimp` |

## Adding Custom Templates

Templates are stored in `web/src/data/templates.json`. To add a custom template:

1. Add a new entry to the `templates` array
2. Required fields:
   - `template_id`: Unique identifier
   - `template_version`: Version string
   - `template_category`: One of the valid categories
   - `name`, `description`, `icon`: Application metadata
   - `category`: Display category name
   - `launch_type`: Must be `"container"` for templates
   - `container_image`: Docker image reference
   - `tags`: Array of searchable tags

Example template:

```json
{
  "template_id": "my-app",
  "template_version": "1.0.0",
  "template_category": "development",
  "name": "My Application",
  "description": "Description of my application",
  "url": "",
  "icon": "https://example.com/icon.png",
  "category": "Development",
  "launch_type": "container",
  "container_image": "myorg/myapp:latest",
  "tags": ["tag1", "tag2"],
  "maintainer": "My Organization",
  "documentation_url": "https://docs.example.com",
  "recommended_limits": {
    "cpu_request": "250m",
    "cpu_limit": "1",
    "memory_request": "256Mi",
    "memory_limit": "1Gi"
  }
}
```

## API Integration

When adding a template to Launchpad, the frontend sends a POST request to
`/api/apps` with the application configuration. The backend should:

1. Validate the application data
2. Ensure the ID is unique
3. Store the application in the configuration
4. Return the created application

## Resource Limits

Templates include recommended resource limits based on typical usage:

- **cpu_request/cpu_limit**: CPU allocation (e.g., "500m" = 0.5 cores)
- **memory_request/memory_limit**: Memory allocation (e.g., "512Mi", "2Gi")

These values can be adjusted per deployment needs.
