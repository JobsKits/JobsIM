// Created by Jobs on 2026年11月18日，星期三.
import { useLayoutEffect } from "react";

export function isRectInVisibleViewport(rect: DOMRect, container: DOMRect): boolean {
  const viewport = window.visualViewport;
  const top = viewport?.offsetTop ?? 0, left = viewport?.offsetLeft ?? 0;
  const bottom = top + (viewport?.height ?? window.innerHeight), right = left + (viewport?.width ?? window.innerWidth);
  return rect.height > 0 && rect.width > 0 && rect.top >= Math.max(top, container.top) && rect.bottom <= Math.min(bottom, container.bottom)
    && rect.left >= Math.max(left, container.left) && rect.right <= Math.min(right, container.right);
}

export function useVisibleViewport() {
  useLayoutEffect(() => {
    const root = document.documentElement;
    const viewport = window.visualViewport;
    let frame = 0;
    const update = () => {
      // Pinch zoom must keep its document layout rather than reflow every gesture.
      if (viewport && viewport.scale !== 1) return;
      const height = viewport?.height ?? window.innerHeight;
      if (!Number.isFinite(height) || height <= 0) return;
      root.style.setProperty("--visible-height", `${height}px`);
      root.style.setProperty("--visible-top", `${Math.max(0, viewport?.offsetTop ?? 0)}px`);
      const editing = document.activeElement?.matches("input, textarea, [contenteditable='true']");
      root.dataset.compact = height < 500 ? "true" : "false";
      // Some mobile browsers shrink both viewports when the keyboard opens.
      root.dataset.keyboard = editing && (window.innerHeight - height > 100 || height < 500) ? "open" : "closed";
    };
    const schedule = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(update);
    };
    update();
    viewport?.addEventListener("resize", schedule);
    viewport?.addEventListener("scroll", schedule);
    window.addEventListener("resize", schedule);
    document.addEventListener("focusin", schedule);
    document.addEventListener("focusout", schedule);
    return () => {
      cancelAnimationFrame(frame);
      viewport?.removeEventListener("resize", schedule);
      viewport?.removeEventListener("scroll", schedule);
      window.removeEventListener("resize", schedule);
      document.removeEventListener("focusin", schedule);
      document.removeEventListener("focusout", schedule);
      root.style.removeProperty("--visible-height");
      root.style.removeProperty("--visible-top");
      delete root.dataset.keyboard;
      delete root.dataset.compact;
    };
  }, []);
}
