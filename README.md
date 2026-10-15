# Poker Planning

Estimation agile en temps réel basée sur la suite de Fibonacci.

## Fonctionnalités

- Création de salon avec lien public
- Votes en direct via WebSocket
- Suite Fibonacci : 0, 1, 2, 3, 5, 8, 13, 21, 34, 55, 89, ?
- Visualisation qui a voté / pas voté
- Révélation des votes par l'admin
- Modification de vote après révélation
- Design responsive

## Utilisation

```bash
task build            # Compiler
task run              # Lancer (port 8080)
PORT=9000 task run    # Lancer sur un port spécifique
task test             # Tests E2E
task clean            # Nettoyer
```

Puis ouvrir http://localhost:8080.

## Stack

- Go 1.22+
- gorilla/websocket
- Templates HTML + CSS vanilla
- Aucune base de données (stockage mémoire)

## Licence

AGPL v3