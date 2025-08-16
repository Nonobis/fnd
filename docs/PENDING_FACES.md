# Module de Gestion des Événements en Attente

## Vue d'ensemble

Le module de gestion des événements en attente permet de stocker automatiquement les images des événements "person" détectés par Frigate lorsque le module de reconnaissance faciale n'est pas configuré ou désactivé. Ces images peuvent ensuite être analysées ultérieurement avec CodeProject.AI pour la reconnaissance faciale.

## Fonctionnalités

### Stockage Automatique
- **Détection automatique** : Les événements de type "person" sont automatiquement stockés quand la reconnaissance faciale est désactivée
- **Organisation structurée** : Les images sont organisées par caméra et par date (`pending_faces/camera/YYYY-MM-DD/`)
- **Métadonnées complètes** : Chaque événement stocke l'ID de l'événement Frigate, la caméra, le timestamp et le statut de traitement

### Interface Web
- **Tableau de bord** : Vue d'ensemble avec statistiques (total, en attente, traités)
- **Filtrage avancé** : Par caméra, statut (en attente/traité), et recherche textuelle
- **Gestion des événements** : Visualisation, traitement avec IA, suppression
- **Nettoyage automatique** : Suppression des événements traités anciens

### Intégration CodeProject.AI
- **Traitement différé** : Analyse faciale des événements stockés
- **Résultats détaillés** : Détection et reconnaissance des visages avec notes
- **Base de données** : Intégration avec la base de données des visages existante

## Architecture

### Composants Principaux

#### PendingFacesManager
```go
type PendingFacesManager struct {
    config           *FNDFacialRecognitionConfiguration
    pendingEvents    []PendingFaceEvent
    pendingEventsPath string
    m                sync.RWMutex
}
```

#### PendingFaceEvent
```go
type PendingFaceEvent struct {
    ID          string    `json:"id"`
    EventID     string    `json:"eventId"`
    Camera      string    `json:"camera"`
    ImagePath   string    `json:"imagePath"`
    Timestamp   time.Time `json:"timestamp"`
    Processed   bool      `json:"processed"`
    ProcessedAt time.Time `json:"processedAt,omitempty"`
    Notes       string    `json:"notes,omitempty"`
}
```

### Structure des Fichiers
```
face_db/
├── faces.json                    # Base de données des visages
├── pending_events.json           # Métadonnées des événements en attente
└── pending_faces/
    └── camera_name/
        └── 2024-01-15/
            ├── 14-30-25_uuid1.jpg
            ├── 15-45-12_uuid2.jpg
            └── ...
```

## Configuration

Le module utilise la configuration existante de reconnaissance faciale avec de nouvelles options pour le traitement automatique :

```json
{
  "facialRecognition": {
    "enabled": false,
    "faceDatabasePath": "face_db",
    "codeProjectAIHost": "localhost",
    "codeProjectAIPort": 32168,
    "pendingFacesAutoProcess": false,
    "pendingFacesInterval": 6
  }
}
```

### Options de Traitement Automatique

- **`pendingFacesAutoProcess`** : Active le traitement automatique des événements en attente (défaut: `false`)
- **`pendingFacesInterval`** : Intervalle en heures entre les traitements automatiques (défaut: `6`, min: `1`, max: `168`)

## API Endpoints

### GET /htmx/pending_faces.html
Page principale de gestion des événements en attente.

### GET /api/pending_faces
Liste des événements avec filtres optionnels :
- `camera` : Filtrer par caméra
- `status` : Filtrer par statut ("pending", "processed", "all")

### GET /api/pending_faces/stats
Statistiques des événements (total, en attente, traités, par caméra).

### POST /api/pending_faces/process/:id
Traiter un événement avec CodeProject.AI.

### DELETE /api/pending_faces/:id
Supprimer un événement et son image.

### GET /api/pending_faces/:id/image
Récupérer l'image d'un événement.

### GET /api/pending_faces/cleanup
Nettoyer les événements traités anciens :
- `days` : Nombre de jours (défaut: 30)

### POST /api/facial-recognition/pending-faces-config
Configurer le traitement automatique des événements en attente :
- `pendingFacesAutoProcess` : Activer/désactiver le traitement automatique
- `pendingFacesInterval` : Intervalle en heures (1-168)

## Utilisation

### 1. Configuration Initiale
1. Désactiver la reconnaissance faciale dans la configuration
2. Le module commence automatiquement à stocker les événements "person"

### 2. Gestion via l'Interface Web
1. Accéder à "Pending Faces" dans le menu
2. Visualiser les événements stockés
3. Filtrer par caméra ou statut
4. Traiter les événements avec CodeProject.AI

### 3. Traitement avec CodeProject.AI
1. Activer la reconnaissance faciale
2. Cliquer sur "Process with AI" pour un événement
3. Consulter les résultats dans les notes de l'événement

### 4. Traitement Automatique
1. Aller dans l'onglet "Pending Faces" de la configuration CodeProject.AI
2. Activer "Auto-Processing"
3. Configurer l'intervalle de traitement (par défaut: 6 heures)
4. Le système traitera automatiquement tous les événements en attente

### 5. Nettoyage
1. Utiliser le bouton "Cleanup" pour supprimer les anciens événements traités
2. Spécifier le nombre de jours de rétention

## Avantages

### Pour l'Utilisateur
- **Pas de perte de données** : Aucun événement "person" n'est perdu
- **Flexibilité** : Traitement différé selon les besoins
- **Organisation** : Interface claire pour gérer les événements
- **Intégration** : Compatible avec l'écosystème existant

### Pour le Système
- **Performance** : Pas d'impact sur les performances en temps réel
- **Ressources** : Utilisation optimisée des ressources CodeProject.AI
- **Évolutivité** : Architecture modulaire et extensible
- **Fiabilité** : Gestion robuste des erreurs et des états

## Cas d'Usage

### Scénario 1 : Configuration Progressive
1. Installation initiale sans CodeProject.AI
2. Stockage automatique des événements "person"
3. Configuration ultérieure de CodeProject.AI
4. Traitement en lot des événements stockés

### Scénario 2 : Maintenance
1. Désactivation temporaire de CodeProject.AI
2. Continuation du stockage des événements
3. Réactivation et traitement des événements en attente

### Scénario 3 : Analyse Rétrospective
1. Stockage continu des événements
2. Analyse périodique des événements non traités
3. Identification de patterns ou d'événements importants

### Scénario 4 : Traitement Automatique
1. Configuration du traitement automatique avec intervalle personnalisé
2. Traitement en arrière-plan sans intervention manuelle
3. Surveillance des logs pour le suivi des traitements
4. Gestion automatique des erreurs et retry

## Limitations

- **Espace disque** : Les images stockées peuvent consommer de l'espace
- **Traitement manuel** : Le traitement avec IA nécessite une intervention manuelle
- **Dépendance** : Nécessite CodeProject.AI pour le traitement final

## Maintenance

### Nettoyage Automatique
- Suppression des événements traités anciens
- Nettoyage des répertoires vides
- Gestion des erreurs de stockage

### Surveillance
- Logs détaillés des opérations
- Statistiques de performance
- Gestion des erreurs de traitement

### Traitement Automatique
- Logs détaillés du traitement automatique
- Statistiques de succès/échecs
- Surveillance de l'intervalle de traitement
- Gestion des erreurs de connexion CodeProject.AI

## Évolutions Futures

- **Traitement automatique** : Traitement en lot programmé
- **Intégration avancée** : Export vers d'autres systèmes
- **Analytics** : Statistiques avancées et rapports
- **API étendue** : Endpoints pour intégration externe
