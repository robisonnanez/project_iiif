import { normalizePageSelection } from "./validation";

describe("normalizePageSelection", () => {
  it("normaliza espacios, rangos y duplicados", () => {
    expect(normalizePageSelection(" 1-3, 3, 5 ", 8)).toBe("1,2,3,5");
  });

  it.each(["0", "3-2", "1,,2", "a", "1-9"])("rechaza %s", (value) => {
    expect(() => normalizePageSelection(value, 6)).toThrow();
  });
});
