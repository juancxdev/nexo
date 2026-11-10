# Nexo Frontend

Shell React/Vite con microfrontends registrados por contrato y un design system atómico compartido.

## Ejecutar

```bash
npm install
npm run dev
```

La aplicación queda en `http://localhost:5174`. Para conectar el backend del Equipo 4:

```bash
VITE_TEAM4_API_URL=http://localhost:8080 npm run dev
```

El backend de Team 4 se ejecuta aparte desde `/Users/juancx/Documents/CX-Developments/SELF/onsite-operations/backend`. La sesión, tenant y capabilities del shell son mocks deliberados en esta base; el Core API los reemplazará en la siguiente integración.

## Microfrontends

Un módulo se registra en `src/platform/microfrontends/registry.tsx` mediante `MicrofrontendManifest`. El shell sólo conoce identidad, navegación, icono, capabilities y loader. El módulo debe exportar un componente que reciba `ModuleContext`; no puede modificar estado global ni importar componentes internos de otro módulo.

El runtime actual usa lazy loading local. La interfaz `load()` deja preparada la sustitución por un remote loader/Module Federation cuando el equipo defina el despliegue independiente.

## Design system

`src/design-system` aplica atomic design:

- `tokens.css`: primitivas, semántica y tokens de componentes.
- `atoms.tsx`: Button, IconButton, Badge, Card, Input y Select.
- `molecules.tsx`: PageHeading, StatCard, EmptyState, ModuleLink y StatusLine.
- `organisms.tsx`: Sidebar y navegación móvil.

Shell y Onsite consumen estos componentes. No agregues colores ni espaciados directamente en un microfrontend.

La integración de Team 4 vive en `src/microfrontends/onsite-operations/OnsiteModule.tsx` y conserva el flujo de check-in, asistencia, reportes y acciones mock de HukID dentro del shell Nexo.

Consulta [docs/microfrontend-contract.md](docs/microfrontend-contract.md) para registrar un nuevo módulo y [docs/design-system.md](docs/design-system.md) para consumir el sistema visual.

## Verificación

```bash
npm run test
npm run build
npm run test:e2e
npm run tokens:validate
```
