> The base of this repo was originally forked from https://github.com/nrmitchi/k8s-controller-sidecars

# Kubernetes Custom Controller - Sidecar Shutdown


## What is this?

This is a custom Kubernetes controller for the purpose of watching running pods, and sending a SIGTERM to sidecar containers when the "main" application container has exited (and the sidecars are the only non-terminated containers).

This is a response to https://github.com/kubernetes/kubernetes/issues/25908.

## Usage

1. Deploy the controller into your cluster.
1. Add the `riskified.com/main_sidecars` annotation to your pods, with a comma-seperated list of main container names.

Example:

```yaml
---
apiVersion: batch/v1beta1
kind: CronJob
metadata:
  name: test-job
spec:
  schedule: "*/5 * * * *"
  startingDeadlineSeconds: 240
  failedJobsHistoryLimit: 5
  successfulJobsHistoryLimit: 1
  concurrencyPolicy: "Replace"
  jobTemplate:
    spec:
      activeDeadlineSeconds: 300 # 5 min
      template:
        metadata:
          annotations:
            riskified.com/main_sidecars: test-job
        spec:
          restartPolicy: Never
          containers:
            - name: test-job
              image: ubuntu:latest
              command: ["sleep", "5"]
            - name: istio-proxy
              image: istio-proxy:latest
            - name: another-sidecar
              image: busybox:lates
```
