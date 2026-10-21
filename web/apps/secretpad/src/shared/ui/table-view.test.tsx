import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, it } from "vitest";
import { I18nProvider } from "../lib/i18n";
import { TableView } from "./table-view";
import { StatusBadge } from "./status-badge";
import { RowActions } from "./row-actions";
beforeEach(() => {
  localStorage.clear();
  localStorage.setItem("secretpad-locale", "zh-CN");
});
function Fixture() {
  return (
    <I18nProvider>
      <TableView id="test">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Count</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {[
              { name: "B", count: 12 },
              { name: "A", count: 2 },
            ].map((item) => (
              <tr key={item.name}>
                <td>{item.name}</td>
                <td>{item.count}</td>
                <td>
                  <button>Open {item.name}</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableView>
    </I18nProvider>
  );
}
it("sorts numeric columns on the loaded page and preserves row actions", () => {
  const { container } = render(<Fixture />);
  fireEvent.click(screen.getByRole("button", { name: /Count/ }));
  expect(
    [...container.querySelectorAll("tbody tr")].map((row) => row.textContent),
  ).toEqual(["A2Open A", "B12Open B"]);
  expect(screen.getByRole("button", { name: "Open A" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /Count/ }));
  expect(container.querySelector("tbody tr")?.textContent).toBe("B12Open B");
});
it("remembers density, hidden columns and width and can reset them", () => {
  const first = render(<Fixture />);
  fireEvent.change(screen.getByRole("combobox", { name: "行高" }), {
    target: { value: "comfortable" },
  });
  fireEvent.click(screen.getByText("列设置"));
  fireEvent.click(screen.getByRole("checkbox", { name: "Count" }));
  fireEvent.change(screen.getByRole("slider", { name: "Name · 列宽" }), {
    target: { value: "300" },
  });
  expect(screen.queryByRole("columnheader", { name: /Count/ })).toBeNull();
  first.unmount();
  const second = render(<Fixture />);
  expect(screen.getByRole("combobox", { name: "行高" })).toHaveProperty(
    "value",
    "comfortable",
  );
  expect(second.container.querySelector("th")?.style.width).toBe("300px");
  fireEvent.click(screen.getByText("列设置"));
  fireEvent.click(screen.getByRole("button", { name: "恢复默认" }));
  expect(screen.getByRole("columnheader", { name: /Count/ })).toBeTruthy();
});
it("translates known states and retains unknown server values", () => {
  render(
    <I18nProvider>
      <StatusBadge value="SUCCEED" />
      <StatusBadge value="CUSTOM_STATE" />
      <RowActions>
        <>
          <button>Primary</button>
          <button>Secondary</button>
        </>
      </RowActions>
    </I18nProvider>,
  );
  expect(screen.getByText("成功")).toBeTruthy();
  expect(screen.getByText("CUSTOM_STATE")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Primary" })).toBeTruthy();
  expect(screen.getByText("更多").closest("details")?.open).toBe(false);
  fireEvent.click(screen.getByText("更多"));
  expect(screen.getByRole("button", { name: "Secondary" })).toBeTruthy();
});

it("does not crash or hide all columns when saved display preferences are malformed", () => {
  localStorage.setItem(
    "privahub-table-test",
    '{"hidden":[0,1,2,999],"widths":{"0":-50},"density":"broken"}',
  );
  render(<Fixture />);
  expect(screen.getByRole("columnheader", { name: /Name/ })).toBeTruthy();
  expect(screen.getByRole("columnheader", { name: /Actions/ })).toBeTruthy();
  expect(screen.getByRole("combobox", { name: "行高" })).toHaveProperty(
    "value",
    "compact",
  );
});
