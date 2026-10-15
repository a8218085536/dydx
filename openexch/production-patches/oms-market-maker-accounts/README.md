# OMS market-maker account production overlay

This overlay adds hybrid Tyche/API-key authentication and trading-only market-maker
authorization without replacing the production-only OMS extensions.

The baseline inspected on 2026-10-15 was the active production JAR at:

```text
/home/ec2-user/flydex/neco-exchange/services/oms-app.jar
SHA-256 d4759692c21d56093d231d0f4441626f201ff07bfeb9f162992270ad18d7e761
```

The baseline contains Tyche authentication, internal NECO deposit credit, direct and
target-chain withdrawal, the unified withdrawal fee, deposit addresses, `/ws/v1`, and
market-order price protection. `RestApiHandler` and `OmsApplication` were reconstructed
from that exact JAR because the clean local OMS source does not contain all deployed
extensions.

The patch changes only these behaviors:

- adds `OMS_AUTH_MODE=tyche-api-key` while retaining Tyche sessions;
- adds read, trade, and funding authorization actions;
- denies every `MARKET_MAKER` principal access to internal credit, manual balance
  movement, direct NECO withdrawal, and target-chain withdrawal;
- permits a temporary `ANY_USER,FUNDING_ADMIN` operator to provision balances;
- preserves the existing internal-key worker and ordinary Tyche withdrawal paths.

Build from the repository root with Java 21:

```powershell
./scripts/build-spot-market-maker-accounts-patch.ps1 `
  -BaseJar ./.codex-tmp/spot-mm-prod-20261015/oms-app-production.jar `
  -OutputJar ./.codex-tmp/spot-mm-prod-20261015/oms-app-market-maker-candidate.jar
```

The build refuses in-place modification, runs the production money-route test, and
compares every non-overlay JAR entry by SHA-256. Secrets are not inputs and are never
packaged.
