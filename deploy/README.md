# Incipit Deployment

## Build and push the image

    docker build -t ghcr.io/superversivesf/incipit:latest .
    docker push ghcr.io/superversivesf/incipit:latest

## Deploy to k3s

    helm upgrade --install incipit veridian-apps/veridian-apps \
      -f deploy/values.yaml \
      -n veridiandynamics

## First-time setup

After deployment, create the database and an admin user:

    kubectl exec -it deployment/incipit -n veridiandynamics -- /incipit init
    kubectl exec -it deployment/incipit -n veridiandynamics -- /incipit add-user --username admin --password 'yourpassword' --role admin

## Backup

The entire system state is in the PVC at /data:
- /data/books.db (SQLite database)
- /data/files/ (EPUB files)
- /data/covers/ (cover images)

Backup with restic or rsync of that directory.

## KOReader configuration

- OPDS catalog URL: https://incipit.veridiandynamics/opds
- Sync server URL: https://incipit.veridiandynamics
- Username/password: created via CLI above