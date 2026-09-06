import { Component, type ErrorInfo, type ReactNode } from "react";
import { Alert, Button, Card } from "./ui";

interface RouteErrorBoundaryProps {
  children: ReactNode;
  routeKey: string;
}

interface RouteErrorBoundaryState {
  error: Error | null;
}

// RouteErrorBoundary aísla errores de render para que una vista defectuosa no desmonte todo el panel.
export class RouteErrorBoundary extends Component<RouteErrorBoundaryProps, RouteErrorBoundaryState> {
  state: RouteErrorBoundaryState = { error: null };

  // getDerivedStateFromError convierte una excepción descendiente en una vista recuperable.
  static getDerivedStateFromError(error: Error): RouteErrorBoundaryState {
    return { error };
  }

  // componentDidCatch registra contexto técnico sin exponer detalles internos en la interfaz.
  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error("Error no controlado en la vista", { route: this.props.routeKey, error, componentStack: info.componentStack });
  }

  // componentDidUpdate restablece el límite cuando el usuario navega a otra vista.
  componentDidUpdate(previousProps: RouteErrorBoundaryProps): void {
    if (previousProps.routeKey !== this.props.routeKey && this.state.error) this.setState({ error: null });
  }

  // render muestra la vista solicitada o una alternativa accesible con recuperación explícita.
  render(): ReactNode {
    if (!this.state.error) return this.props.children;
    return <Card className="route-error">
      <Alert tone="danger">Esta sección encontró un error inesperado, pero el panel continúa disponible.</Alert>
      <p>Recarga la sección. Si el problema continúa, informa la ruta y la hora al administrador.</p>
      <Button onClick={() => window.location.reload()}>Recargar sección</Button>
    </Card>;
  }
}
