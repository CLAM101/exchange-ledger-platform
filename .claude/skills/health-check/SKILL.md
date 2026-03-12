---
name: health-check
description: Check health status of all exchange-ledger-platform services
disable-model-invocation: false
user-invokable: true
---

# Health Check

Check the health and status of all running services.

## Instructions

1. Check if Docker containers are running:
   ```bash
   docker-compose ps
   ```

2. Check each service's metrics endpoint to verify they're responding:
   - Ledger: http://localhost:9091/metrics
   - Account: http://localhost:9092/metrics
   - Wallet: http://localhost:9093/metrics
   - Asset: http://localhost:9095/metrics
   - Gateway: http://localhost:9094/metrics

3. Check infrastructure services:
   - MySQL: Port 3306
   - Redis: Port 6379
   - Prometheus: http://localhost:9090
   - Grafana: http://localhost:3000

4. Report status summary:
   - Which services are healthy
   - Which services are down or unhealthy
   - Any error messages from docker logs if services are failing

## Quick Commands

```bash
# Check container status
docker-compose ps

# Check service logs for errors
docker-compose logs --tail=20 ledger account wallet gateway

# Test metrics endpoints
curl -s http://localhost:9091/metrics | head -5
```
