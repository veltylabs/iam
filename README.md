# iam
<img src="docs/img/badges.svg">

Servicio central de identidad, sesión SSO y RBAC para todos los proyectos de
Velty. Reemplaza el patrón actual — cada app monta su propio
`tinywasm/auth` + `tinywasm/rbac` contra su propia base — por un servicio
único que las apps consumidoras llaman por HTTP.

Motor: usa `tinywasm/user` + `tinywasm/auth` + `tinywasm/rbac` sin
modificarlas (son librerías genéricas ya publicadas, compartidas con
cualquier otro consumidor del ecosistema). `iam` es la nueva raíz de
composición que hoy vive duplicada — y desincronizada — en cada app: ver
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) §1 para la evidencia concreta.

```mermaid
flowchart TD
    U[tinywasm/user] --> A[tinywasm/auth]
    U --> R[tinywasm/rbac]
    A --> I[veltylabs/iam]
    R --> I
    I -->|HTTP + Bearer, Etapa 3| M[misitio]
    I -->|HTTP + Bearer, Etapa 3| O[futuros proyectos Velty]
```

## Documentación

- [Arquitectura](docs/ARCHITECTURE.md) — por qué existe, límites de
  responsabilidad frente a `site_manager`, decisiones tomadas y lo que
  queda deliberadamente abierto.
- [Plan](docs/PLAN.md) — etapas de implementación.

## Estado

Etapa 1 en curso: portar el motor de identidad+RBAC desde
`veltylabs/misitio`. Todavía no expone API HTTP (Etapa 3) ni sesión SSO
cross-dominio (Etapa 4) — ver [`docs/PLAN.md`](docs/PLAN.md).
