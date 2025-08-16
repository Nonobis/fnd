## Version 0.1.16 - 16.08.2025

### Changes since v0.1.15

- Standardiser les chemins de stockage - tous les fichiers JSON et DB dans fnd_conf/
- Corriger les problèmes de la page Task Scheduler - onglets et boutons d'exécution
- update gitignore
- Corriger le crash de la page Task Scheduler - utiliser template.ParseFS au lieu de c.HTML
- Implémenter le système de tâches planifiées avec file d'attente d'événements et interface de gestion
- Merge branch 'main' of https://github.com/Nonobis/fnd
- Corriger l'initialisation des onglets sur la page des templates de notification

### 🐳 Docker Image

This release is available as a Docker image:

**Main image:**
```bash
docker pull ghcr.io/Nonobis/fnd:0.1.16
```

**Latest image:**
```bash
docker pull ghcr.io/Nonobis/fnd:latest
```

**Direct links:**
- 📦 [GitHub Package](https://github.com/Nonobis/fnd/pkgs/container/fnd)
- 🏷️ [Tag 0.1.16](ghcr.io/Nonobis/fnd:0.1.16)

## Version 0.1.15 - 16.08.2025

### Changes since v0.1.13

- Merge branch 'main' of https://github.com/Nonobis/fnd
- Corriger l'initialisation du système d'onglets sur la page de configuration faciale
- Corriger l'initialisation du système d'onglets sur la page de configuration faciale
- Améliorer l'interface utilisateur : aligner le design de la page pending faces et corriger la lisibilité de la popup Object Filters
- Fix runtime panic in background task when pending faces auto-process is disabled
- Fix runtime panic in background task when pending faces auto-process is disabled
- feat: add scheduled automatic processing for pending face events with configurable interval and comprehensive logging
- feat: add scheduled automatic processing for pending face events with configurable interval and comprehensive logging
- chore: bump version to 0.1.14 and update changelog
- feat : fix build
- feat: make MQTT client ID and topic prefix configurable with sensible defaults
- feat: enhance MQTT logging with detailed debug information for better event capture diagnostics
- Merge branch 'main' of https://github.com/Nonobis/fnd
- remove markdown
- feat: condenser davantage le formulaire MQTT en supprimant les sections
- feat: condenser le formulaire MQTT pour éviter les ascenseurs
- feat: ajouter validation des formulaires avec contrôle client-side
- feat : fix objects filters
- feat: align modal popups with light theme design
- add logs
- feat: add tabs to notification templates and fix preview functionality
- fix: align facial recognition status card with overview design

### 🐳 Docker Image

This release is available as a Docker image:

**Main image:**
```bash
docker pull ghcr.io/Nonobis/fnd:0.1.15
```

**Latest image:**
```bash
docker pull ghcr.io/Nonobis/fnd:latest
```

**Direct links:**
- 📦 [GitHub Package](https://github.com/Nonobis/fnd/pkgs/container/fnd)
- 🏷️ [Tag 0.1.15](ghcr.io/Nonobis/fnd:0.1.15)

## Version 0.1.14 - 16.08.2025

### Changes since v0.1.13

- feat : fix build
- feat: make MQTT client ID and topic prefix configurable with sensible defaults
- feat: enhance MQTT logging with detailed debug information for better event capture diagnostics
- Merge branch 'main' of https://github.com/Nonobis/fnd
- remove markdown
- feat: condenser davantage le formulaire MQTT en supprimant les sections
- feat: condenser le formulaire MQTT pour éviter les ascenseurs
- feat: ajouter validation des formulaires avec contrôle client-side
- feat : fix objects filters
- feat: align modal popups with light theme design
- add logs
- feat: add tabs to notification templates and fix preview functionality
- fix: align facial recognition status card with overview design

### 🐳 Docker Image

This release is available as a Docker image:

**Main image:**
```bash
docker pull ghcr.io/Nonobis/fnd:0.1.14
```

**Latest image:**
```bash
docker pull ghcr.io/Nonobis/fnd:latest
```

**Direct links:**
- 📦 [GitHub Package](https://github.com/Nonobis/fnd/pkgs/container/fnd)
- 🏷️ [Tag 0.1.14](ghcr.io/Nonobis/fnd:0.1.14)

## Version 0.1.13 - 16.08.2025

### Changes since v0.1.11

- Merge branch 'main' of https://github.com/Nonobis/fnd
- fix: ensure latest tag matches version SHA by using metadata action
- fix: use step outputs for cross-step variable access
- feat: translate release notes to English
- chore: bump version to 0.1.12 and update changelog
- fix: convert repository name to lowercase for Docker tagging
- Merge branches 'main' and 'main' of https://github.com/Nonobis/fnd
- feat: make latest tag dependent on successful build

### 🐳 Docker Image

This release is available as a Docker image:

**Main image:**
```bash
docker pull ghcr.io/Nonobis/fnd:0.1.13
```

**Latest image:**
```bash
docker pull ghcr.io/Nonobis/fnd:latest
```

**Direct links:**
- 📦 [GitHub Package](https://github.com/Nonobis/fnd/pkgs/container/fnd)
- 🏷️ [Tag 0.1.13](ghcr.io/Nonobis/fnd:0.1.13)

## Version 0.1.12 - 16.08.2025

### Changements depuis v0.1.11

- fix: convert repository name to lowercase for Docker tagging
- Merge branches 'main' and 'main' of https://github.com/Nonobis/fnd
- feat: make latest tag dependent on successful build

### 🐳 Image Docker

Cette release est disponible sous forme d'image Docker :

**Image principale :**
```bash
docker pull ghcr.io/Nonobis/fnd:0.1.12
```

**Image latest :**
```bash
docker pull ghcr.io/Nonobis/fnd:latest
```

**Liens directs :**
- 📦 [Package GitHub](https://github.com/Nonobis/fnd/pkgs/container/fnd)
- 🏷️ [Tag 0.1.12](ghcr.io/Nonobis/fnd:0.1.12)

## Version 0.1.11 - 16.08.2025

### Changements depuis v0.1.10

- WIP : First version CodeServer.AI support (Face recognition)
- update todo
- build started on main commit
- add more logs
- Suppression du bouton de test de logging inutile

### 🐳 Image Docker

Cette release est disponible sous forme d'image Docker :

**Image principale :**
```bash
docker pull ghcr.io/Nonobis/fnd:0.1.11
```

**Image latest :**
```bash
docker pull ghcr.io/Nonobis/fnd:latest
```

**Liens directs :**
- 📦 [Package GitHub](https://github.com/Nonobis/fnd/pkgs/container/fnd)
- 🏷️ [Tag 0.1.11](ghcr.io/Nonobis/fnd:0.1.11)

## Version 0.1.10 - 15.08.2025

### Changements depuis v0.1.8

- Feature/gitea (#7)

### 🐳 Image Docker

Cette release est disponible sous forme d'image Docker :

**Image principale :**
```bash
docker pull ghcr.io/Nonobis/fnd:0.1.10
```

**Image latest :**
```bash
docker pull ghcr.io/Nonobis/fnd:latest
```

**Liens directs :**
- 📦 [Package GitHub](https://github.com/Nonobis/fnd/pkgs/container/fnd)
- 🏷️ [Tag 0.1.10](ghcr.io/Nonobis/fnd:0.1.10)

## Version 0.1.8 - 15.08.2025

### Changements depuis v0.1.7

- UI & Notification System Improvements + Logging Support (#4)

### 🐳 Image Docker

Cette release est disponible sous forme d'image Docker :

**Image principale :**
```bash
docker pull ghcr.io/Nonobis/fnd:0.1.8
```

**Image latest :**
```bash
docker pull ghcr.io/Nonobis/fnd:latest
```

**Liens directs :**
- 📦 [Package GitHub](https://github.com/Nonobis/fnd/pkgs/container/fnd)
- 🏷️ [Tag 0.1.8](ghcr.io/Nonobis/fnd:0.1.8)
