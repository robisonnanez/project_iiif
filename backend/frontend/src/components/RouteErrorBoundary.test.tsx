import { render, screen } from "@testing-library/react";
import { vi } from "vitest";
import { RouteErrorBoundary } from "./RouteErrorBoundary";

function BrokenView() {
  throw new Error("fallo controlado de prueba");
}

it("mantiene el panel disponible cuando una vista falla durante el render", () => {
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  render(<RouteErrorBoundary routeKey="config"><BrokenView /></RouteErrorBoundary>);
  expect(screen.getByText(/el panel continúa disponible/i)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Recargar sección" })).toBeInTheDocument();
});
