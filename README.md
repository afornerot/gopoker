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

## Démarrage avec le binaire

Le binaire `gopoker` est un exécutable autonome : pas de dépendance, pas d'installation.

**Téléchargement** : les binaires des releases sont disponibles sur
<https://github.com/afornerot/gopoker/releases> (choisir la version, puis le
binaire correspondant à sa plateforme, ex. `gopoker-linux-amd64`).
Sur Linux/macOS, rendre le fichier exécutable au besoin : `chmod +x gopoker-linux-amd64`.
Pour un binaire au fil de l'eau, compiler soi-même depuis le dépôt (voir plus bas).

**Premier plan** (port 8080 par défaut) :

```bash
./gopoker
# → Serveur démarré sur :8080  →  http://localhost:8080
```

**Changer le port** : la variable d'environnement `PORT` est lue au démarrage.

```bash
PORT=9000 ./gopoker     # → http://localhost:9000
```

**Mode daemon** (arrière-plan, logs dans `gopoker.log`) :

```bash
./gopoker > gopoker.log 2>&1 &
echo $!                  # affiche le PID à retenir
tail -f gopoker.log      # suivre les logs
```

**Daemon avec port personnalisé** :

```bash
PORT=9000 ./gopoker > gopoker.log 2>&1 &
```

**Arrêter le daemon** :

```bash
kill <PID>               # par son PID
pkill -f gopoker         # ou par nom de process
```

## Démarrage avec le dépôt

Depuis un clone du dépôt, `task` (go-task) gère le build et le lancement.

```bash
git clone https://github.com/afornerot/gopoker.git
cd gopoker
task build        # compile le binaire gopoker
task run          # lance (port 8080)
PORT=9000 task run
```

### Tâches disponibles

```bash
task run-daemon       # compile + lance en arrière-plan (logs: gopoker.log)
task stop-daemon      # arrête le daemon
task build            # compile le binaire gopoker
task dev              # compile puis lance
task test             # tests E2E
task test-short       # tests E2E rapide (sans cache)
task clean            # supprime le binaire
```

## Tests

```bash
task test             # tests E2E (création salon, votes, révélation, admin)
```

## Stack

- Go 1.22+
- gorilla/websocket
- Templates HTML + CSS vanilla
- Aucune base de données (stockage mémoire)

## Licence

AGPL v3