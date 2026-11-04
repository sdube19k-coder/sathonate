# Credential rotation and recovery

## Implemented boundary

Administrator-only API: `POST /api/v1/deployments/{operationId}/rotation-plans` with an `Idempotency-Key` and body `{"identityRef":"org1/peer1"}`; `GET /api/v1/deployments/{operationId}/rotation-plans/{planId}` retrieves the same tenant/network/target-bound record. Only identities in the recorded topology are accepted. Run the API's explicit migration command before using the new maintenance table. No live mutator or HTTP checkpoint/verification endpoint is registered. A plan is not a rotated identity.

The PostgreSQL journal has tenant keys, RLS policies, unique idempotency hashes, a binding hash, revision compare-and-swap and fixed checkpoint metadata. It stores no passwords or private keys. Unit tests use explicitly simulated driver receipts; simulated and real receipts cannot mix. The real PostgreSQL test validates duplicate creation, wrong binding/tenant refusal, recovery and stale revision rejection. It does not prove actual Fabric renewal or revocation.

Stages are inventory, backup, replacement enrollment, replacement verification, activation, active verification and retirement. Every step needs an administrator approval bound to the intent, identity/network/target digest and exact step, expiring within one hour. A future trusted driver must reconcile the same `intentID/step` key after interruption; never blindly repeat a CA identity update. Unverified receipts cannot advance, and a missing live driver fails closed. Clients cannot mark a certificate verified by posting a boolean.

## Operator procedure before any live adapter is enabled

1. Verify the deployment target, namespace UID, identity-to-node mapping and current network health. Privately inventory enrollment Secret references, CA registry/CR fields, wallet references, MSP/TLS certificate fingerprints and expiration, Helm releases/history, PVCs, CA databases, clients, channel identities and external backups. Do not export Secret payloads into evidence.
2. Obtain an explicit maintenance change and per-step approval. Do not delete immutable enrollment Secrets or rotate because a read failed.
3. Quiesce/freeze the appropriate state, create protected backups through an approved process, retain the encryption key independently, and prove restoration in a disposable environment. A byte-bundle encryption test is not a consistent ledger/CA/PostgreSQL backup test.
4. Register a distinct replacement enrollment identity using protected input. Preserve old credentials and CA database state while enrolling replacement signing/TLS certificates. Verify trust chains, SANs, MSP/OU membership, certificate/key consistency, expiry and consumer compatibility before activation. Signing/TLS CA roles are different; certificate substitution cannot be guessed.
5. Activate only through a tested supported controller/pre-enrollment design. For three Raft nodes change one node at a time with quorum and channel-consenter TLS update procedures. Verify both organizations' endorsement/commit/query and every affected peer/orderer/client/chaincode after each change. This controller migration is still unqualified.
6. If interrupted, load the durable intent, compare exact binding and revision, inspect actual completed effects, and reconcile the same step key. A failed replacement/active verification forbids retirement. Preserve evidence of both old and replacement references privately for recovery; never overwrite or fabricate a success receipt.
7. Retire/revoke only after explicit separate approval, verified active behavior and a tested rollback/recovery policy. The fresh-network registrar intentionally lacks revocation/delegation privileges; a separately approved revoker identity and CRL distribution design are required. No such identity is automatically granted here.
8. Independently verify removal/retention of old CR/Helm/audit/backup copies. Revocation is not erasure, and rotating a password does not revoke existing certificates.

## Recovery limits

The maintenance coordinator and PostgreSQL checkpoints are implemented and tested; the live Fabric rotation/revocation driver is deliberately unconfigured. Do not call this a complete rotation system. Existing networks are never changed by creating/retrieving plans. A database administrator can bypass application isolation; a future driver must be reviewed as privileged code. Production remains disabled.
