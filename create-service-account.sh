#!/bin/bash
set -ex

PROJECT_ID=$(gcloud config get-value project)
SA_NAME="run-otel-example-sa"

#### Create service account with required roles
gcloud iam service-accounts create "${SA_NAME}" \
  --description="A service account just to used for Cloud Run observability demo. https://github.com/GoogleCloudPlatform/opentelemetry-cloud-run" \
  --display-name="Cloud Run OpenTelemetry demo service account" \
  --quiet

gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/iam.serviceAccountUser" \
  --quiet

gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/storage.objectViewer" \
  --quiet

gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/logging.logWriter" \
  --quiet

gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/artifactregistry.createOnPushWriter" \
  --quiet

# In order to change policy of the run service, it requires 'run.services.setIamPolicy',
# which is contained in run.admin role
gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/run.admin" \
  --quiet
