import { onBeforeUnmount, onMounted, ref, type Ref } from "vue";

/**
 * Polls fetcher immediately and then every intervalMs, exposing the last
 * successful result. A failed poll (e.g. the daemon briefly unreachable)
 * leaves the previous value in place rather than clearing it, so a
 * transient hiccup doesn't flash the UI to empty.
 */
export function usePolling<T>(fetcher: () => Promise<T>, intervalMs: number): { data: Ref<T | null>; error: Ref<string | null>; refresh: () => Promise<void> } {
  const data = ref<T | null>(null) as Ref<T | null>;
  const error = ref<string | null>(null);
  let timer: ReturnType<typeof setInterval> | undefined;

  async function refresh() {
    try {
      data.value = await fetcher();
      error.value = null;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    }
  }

  onMounted(() => {
    refresh();
    timer = setInterval(refresh, intervalMs);
  });
  onBeforeUnmount(() => {
    if (timer) clearInterval(timer);
  });

  return { data, error, refresh };
}
