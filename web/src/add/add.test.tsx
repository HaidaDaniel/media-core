// The quick-add screen: title-first manual addition with the server-side
// identity decision — an exact reference opens the existing record without
// writing, possible matches are shown with evidence and nothing is merged or
// created until the user chooses, and a ready result creates directly
// offline. Input is retained on every failure; an ambiguous create failure
// is retried with the same idempotency key, and a stale candidate decision
// re-resolves instead of guessing.
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { JSX } from "react";
import { MemoryRouter, Route, Routes, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { QuickAddPage } from "./QuickAddPage";
import type { AdditionResultWire, ResolveWire } from "./addApi";

const { mockResolve, mockCreateWork, mockAddEdition } = vi.hoisted(() => ({
  mockResolve: vi.fn(),
  mockCreateWork: vi.fn(),
  mockAddEdition: vi.fn(),
}));

vi.mock("./addApi", () => ({
  resolveAddition: (...args: unknown[]) => mockResolve(...args),
  createWork: (...args: unknown[]) => mockCreateWork(...args),
  addEdition: (...args: unknown[]) => mockAddEdition(...args),
  targetPathFor: (r: { edition_id: string | null; work_id: string }) =>
    r.edition_id ? `/editions/${r.edition_id}` : `/works/${r.work_id}`,
  existingWorkPath: (id: string) => `/works/${id}`,
}));

function resolveOutcome(overrides: Partial<ResolveWire>): ResolveWire {
  return {
    outcome: "ready",
    work_id: null,
    candidates: null,
    candidate_token: null,
    request_fingerprint: null,
    ...overrides,
  };
}

function created(workId: string, editionId: string | null = null): AdditionResultWire {
  return { created: true, work_id: workId, work_revision: 1, edition_id: editionId };
}

function WorkProbe(): JSX.Element {
  const { id } = useParams();
  return <p data-testid="probe">work-{id}</p>;
}

function EditionProbe(): JSX.Element {
  const { id } = useParams();
  return <p data-testid="probe">edition-{id}</p>;
}

function renderAdd(): void {
  render(
    <MemoryRouter initialEntries={["/add"]}>
      <Routes>
        <Route path="/add" element={<QuickAddPage />} />
        <Route path="/works/:id" element={<WorkProbe />} />
        <Route path="/editions/:id" element={<EditionProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

async function submitTitle(value = "Neon Garden"): Promise<void> {
  fireEvent.change(screen.getByLabelText("Title"), { target: { value } });
  fireEvent.click(screen.getByRole("button", { name: "Add" }));
}

beforeEach(() => {
  // resetAllMocks (not clearAllMocks) also drops any queued mock*Once
  // implementations a failed test may have left unconsumed.
  vi.resetAllMocks();
});

afterEach(() => {
  cleanup();
});

describe("quick add outcomes", () => {
  it("completes a title-only addition offline and navigates to the new edition", async () => {
    mockResolve.mockResolvedValueOnce(resolveOutcome({ outcome: "ready" }));
    mockCreateWork.mockResolvedValueOnce(created("w-9", "e-9"));
    renderAdd();
    await submitTitle();

    expect((await screen.findByTestId("probe")).textContent).toBe("edition-e-9");
    expect(mockCreateWork).toHaveBeenCalledWith(
      expect.objectContaining({
        titles: [expect.objectContaining({ text: "Neon Garden" })],
        references: [],
      }),
      expect.objectContaining({ requestId: expect.any(String) }),
    );
  });

  it("refuses an empty form without any request", async () => {
    renderAdd();
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toContain("title is required");
    expect(mockResolve).not.toHaveBeenCalled();
    expect(mockCreateWork).not.toHaveBeenCalled();
  });

  it("navigates an existing identity to the existing record without a duplicate", async () => {
    mockResolve.mockResolvedValueOnce(resolveOutcome({ outcome: "existing", work_id: "w-1" }));
    renderAdd();
    fireEvent.change(screen.getByLabelText("URL"), {
      target: { value: "https://example.com/work" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    expect(await screen.findByText(/already exists/)).not.toBeNull();
    expect(mockCreateWork).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Open existing record" }));
    expect((await screen.findByTestId("probe")).textContent).toBe("work-w-1");
  });

  it("shows candidates with evidence and creates nothing until the user chooses", async () => {
    const candidates = [
      {
        work_id: "w-2",
        edition_id: null,
        evidence: ['work title "Neon Garden" (manga)'],
      },
    ];
    mockResolve.mockResolvedValueOnce(
      resolveOutcome({ outcome: "candidates", candidates, candidate_token: "tok" }),
    );
    mockCreateWork.mockResolvedValueOnce(created("w-3"));
    renderAdd();
    await submitTitle();

    expect(await screen.findByText(/Possible matches/)).not.toBeNull();
    expect(mockCreateWork).not.toHaveBeenCalled();
    expect(mockAddEdition).not.toHaveBeenCalled();
    expect(screen.getByText('work title "Neon Garden" (manga)')).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Create as a distinct work" }));
    expect((await screen.findByTestId("probe")).textContent).toBe("work-w-3");
    expect(mockCreateWork).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ choice: "create_distinct", candidateToken: "tok" }),
    );
  });

  it("re-resolves when the medium changes and adds the edition with the fresh token", async () => {
    const candidates = [
      { work_id: "w-2", edition_id: null, evidence: ["edition title match (manga)"] },
    ];
    mockResolve.mockResolvedValueOnce(
      resolveOutcome({ outcome: "candidates", candidates, candidate_token: "tok-old" }),
    );
    mockResolve.mockResolvedValueOnce(
      resolveOutcome({ outcome: "candidates", candidates, candidate_token: "tok-new" }),
    );
    mockAddEdition.mockResolvedValueOnce(created("w-2", "e-7"));
    renderAdd();
    await submitTitle();
    expect(await screen.findByText(/Possible matches/)).not.toBeNull();

    let addEditionButton = screen.getByRole("button", {
      name: "Add edition under this work",
    }) as HTMLButtonElement;
    expect(addEditionButton.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Medium (optional; required to add an edition)"), {
      target: { value: "manga" },
    });
    // The medium change invalidates the token's fingerprint, so the screen
    // re-resolves before the choice is offered again.
    await vi.waitFor(() => expect(mockResolve).toHaveBeenCalledTimes(2));
    expect(mockResolve).toHaveBeenLastCalledWith(expect.objectContaining({ kind: "manga" }));
    addEditionButton = screen.getByRole("button", {
      name: "Add edition under this work",
    }) as HTMLButtonElement;
    expect(addEditionButton.disabled).toBe(false);

    fireEvent.click(addEditionButton);
    expect((await screen.findByTestId("probe")).textContent).toBe("edition-e-7");
    expect(mockAddEdition).toHaveBeenCalledWith(
      "w-2",
      expect.anything(),
      expect.objectContaining({ candidateToken: "tok-new" }),
    );
  });

  it("resolves a URL-only entry to the existing record without sending a title", async () => {
    mockResolve.mockResolvedValueOnce(resolveOutcome({ outcome: "existing", work_id: "w-6" }));
    renderAdd();
    fireEvent.change(screen.getByLabelText("URL"), {
      target: { value: "https://example.com/x" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    expect(await screen.findByText(/already exists/)).not.toBeNull();
    expect(mockResolve).toHaveBeenCalledWith(
      expect.objectContaining({
        titles: [],
        references: [{ url: "https://example.com/x" }],
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Open existing record" }));
    expect((await screen.findByTestId("probe")).textContent).toBe("work-w-6");
  });

  it("reports the server's manual-title requirement for an unknown URL and keeps the input", async () => {
    mockResolve.mockRejectedValueOnce(
      new ApiError(
        "invalid_argument",
        400,
        "a title is required when the identity is not known",
        false,
      ),
    );
    renderAdd();
    fireEvent.change(screen.getByLabelText("URL"), {
      target: { value: "https://example.com/x" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("title is required when the identity is not known");
    // The reference is retained for the manual-title retry.
    expect((screen.getByLabelText("URL") as HTMLInputElement).value).toBe(
      "https://example.com/x",
    );
  });

  it("sends both a URL and an external ID when both are filled", async () => {
    mockResolve.mockResolvedValueOnce(resolveOutcome({ outcome: "ready" }));
    mockCreateWork.mockResolvedValueOnce(created("w-8"));
    renderAdd();
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "T" } });
    fireEvent.change(screen.getByLabelText("URL"), {
      target: { value: "https://example.com/x" },
    });
    fireEvent.change(screen.getByLabelText("Provider (optional, with external ID)"), {
      target: { value: "prov" },
    });
    fireEvent.change(screen.getByLabelText("External ID (optional, with provider)"), {
      target: { value: "123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    expect(await screen.findByTestId("probe")).not.toBeNull();
    expect(mockResolve).toHaveBeenCalledWith(
      expect.objectContaining({
        references: [
          { url: "https://example.com/x" },
          { provider: "prov", external_id: "123" },
        ],
      }),
    );
  });
});

describe("quick add failures", () => {
  it("retries an ambiguous create failure with the same idempotency key and input", async () => {
    mockResolve.mockResolvedValueOnce(resolveOutcome({ outcome: "ready" }));
    mockCreateWork.mockRejectedValueOnce(new ApiError("locked", 503, "the storage is locked", true));
    mockCreateWork.mockResolvedValueOnce(created("w-4"));
    renderAdd();
    await submitTitle();

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the storage is locked");
    // The input is retained on failure.
    expect((screen.getByLabelText("Title") as HTMLInputElement).value).toBe("Neon Garden");

    const first = mockCreateWork.mock.calls[0][1] as { requestId: string };
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect((await screen.findByTestId("probe")).textContent).toBe("work-w-4");
    const second = mockCreateWork.mock.calls[1][1] as { requestId: string };
    expect(second.requestId).toBe(first.requestId);
  });

  it("re-resolves a stale candidate decision and keeps the input", async () => {
    const candidates = [{ work_id: "w-2", edition_id: null, evidence: ["evidence one"] }];
    mockResolve.mockResolvedValueOnce(
      resolveOutcome({ outcome: "candidates", candidates, candidate_token: "tok-old" }),
    );
    mockCreateWork.mockRejectedValueOnce(
      new ApiError("conflict", 409, "the candidate decision expired; resolve again", false),
    );
    renderAdd();
    await submitTitle();
    expect(await screen.findByText(/Possible matches/)).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Create as a distinct work" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("expired");
    // The input is retained across the failure.
    expect((screen.getByLabelText("Title") as HTMLInputElement).value).toBe("Neon Garden");

    mockResolve.mockResolvedValueOnce(
      resolveOutcome({ outcome: "candidates", candidates, candidate_token: "tok-new" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Resolve again" }));
    await screen.findByText(/Possible matches/);

    mockCreateWork.mockResolvedValueOnce(created("w-5"));
    fireEvent.click(screen.getByRole("button", { name: "Create as a distinct work" }));
    expect((await screen.findByTestId("probe")).textContent).toBe("work-w-5");
    const retried = mockCreateWork.mock.calls[1][1] as { candidateToken?: string };
    expect(retried.candidateToken).toBe("tok-new");
  });

  it("shows a resolve failure as a distinguishable error and keeps the form", async () => {
    mockResolve.mockRejectedValueOnce(
      new ApiError("invalid_argument", 400, "a reference requires exactly one of external_id or url", false),
    );
    renderAdd();
    fireEvent.change(screen.getByLabelText("URL"), { target: { value: "https://example.com" } });
    fireEvent.change(screen.getByLabelText("External ID (optional, with provider)"), {
      target: { value: "123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("exactly one");
    expect((screen.getByLabelText("URL") as HTMLInputElement).value).toBe(
      "https://example.com",
    );
    expect(mockCreateWork).not.toHaveBeenCalled();
  });
});
