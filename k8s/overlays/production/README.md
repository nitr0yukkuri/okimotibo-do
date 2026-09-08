# Production template

The repository root `k8s/` is the anonymous, single-replica kind environment. Do not apply it to production.

This overlay is a production-shaped template. Before applying it:

1. Replace `frontend.example.com` and `api.example.com` in the patch and Ingress files.
2. Replace the `:stable` image names with images from the actual registry.
3. Install an Ingress controller and Metrics Server.
4. Create the required Secret without committing it:

```powershell
kubectl -n okimochi create secret generic okimochi-backend-secret `
  --from-literal=SUPABASE_URL="https://your-project.supabase.co" `
  --from-literal=SUPABASE_SECRET_KEY="your-secret-key" `
  --from-literal=REDIS_URL="rediss://your-managed-redis:6380"
```

5. Create `okimochi-api-tls` through the cluster's certificate process.
6. Render and inspect before applying:

```powershell
kubectl kustomize .\k8s\overlays\production
kubectl apply -k .\k8s\overlays\production
```

This overlay deliberately does not deploy the local, non-persistent Redis. Use a managed Redis or add a separately reviewed HA Redis topology. The Backend must use Supabase (or another shared store) before running multiple replicas.
