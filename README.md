# Gopoker Planning

Estimation agile en temps réel basée sur la suite de Fibonacci, en Go.

## Fonctionnalités

- Création d'un salon nommé avec lien public à partager
- Page d'accueil listant les salons en cours
- Votes en direct via WebSocket (temps réel)
- Suite Fibonacci : `0, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89, ?`
- Visualisation en direct de qui a voté / pas voté (pastilles couleur)
- Panneau de synthèse : barres par score + tags des votants sous chaque barre
- Révélation des votes décidée par l'admin, modification du vote après révélation possible
- Gestion admin : suppression d'un participant déconnecté, transfert de l'admin (couronne 👑), auto-promotion du premier connecté si l'admin quitte
- Redirection auto vers l'accueil depuis une URL de salon périmée
- Design responsive, notifications toasts, reconnexion automatique

## Démarrage

```bash
task run              # Lance sur http://localhost:8080
PORT=9000 task run    # Port personnalisé
```

Ou directement : `./gopoker`.

## Tâches

```bash
task run-daemon       # Compile + lance en arrière-plan (logs: gopoker.log)
task stop-daemon      # Arrête le daemon
task build            # Compile le binaire gopoker
task dev              # Compile puis lance
task test             # Tests E2E
task test-short       # Tests E2E rapide (sans cache)
task clean            # Supprime le binaire
```

## Release GitHub

Un workflow `.github/workflows/release.yml` compile les binaires pour
`linux / darwin / windows` en `amd64` et `arm64` et publie une release GitHub
à chaque tag `v*` :

```bash
git tag v1.0
git push origin v1.0
```

## Stack

- Go 1.22+
- gorilla/websocket
- Templates HTML + CSS vanilla
- Aucune base de données (stockage mémoire)

## Licence

AGPL v3