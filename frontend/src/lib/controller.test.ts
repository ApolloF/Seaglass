import { describe, expect, it } from "vitest";
import { controllerSummary } from "./controller";
import type { ControllerStatus } from "./types";

const none: ControllerStatus = { viGEm: false, hidHide: false, hidHideActive: false, canHide: false, restart: false, hidden: false };

describe("controllerSummary", () => {
  it("waits for the status", () => {
    expect(controllerSummary(null)).toMatchObject({ detail: "Checking…", canSetUp: false });
  });

  it("asks for the drivers when they are missing", () => {
    expect(controllerSummary(none)).toEqual({ detail: "Needs two small drivers (ViGEmBus and HidHide).", ok: false, canSetUp: true });
  });

  it("says when Windows must restart, with nothing more to set up", () => {
    expect(controllerSummary({ ...none, viGEm: true, restart: true })).toEqual({ detail: "Restart Windows to finish setting up.", ok: false, canSetUp: false });
  });

  it("offers to install HidHide when only it is missing", () => {
    expect(controllerSummary({ ...none, viGEm: true })).toEqual({
      detail: "Ready. Your controller can't be hidden from games yet.",
      ok: true,
      canSetUp: true,
    });
  });

  it("is ready and done when the controller can be hidden too", () => {
    expect(controllerSummary({ ...none, viGEm: true, hidHide: true, hidHideActive: true, canHide: true })).toEqual({ detail: "Ready.", ok: true, canSetUp: false });
  });

  it("says how to switch hiding on when HidHide is there but off", () => {
    expect(controllerSummary({ ...none, viGEm: true, hidHide: true })).toEqual({
      detail: "Ready. Turn on hiding in HidHide so games don't see your controller twice.",
      ok: true,
      canSetUp: false,
    });
  });
});
