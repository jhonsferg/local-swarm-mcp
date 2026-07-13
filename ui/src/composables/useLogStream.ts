import { onBeforeUnmount, onMounted, ref, type Ref } from "vue";

/** Subscribes to the daemon's live log tail (Server-Sent Events). */
export function useLogStream(maxLines = 500): { lines: Ref<string[]> } {
  const lines = ref<string[]>([]) as Ref<string[]>;
  let source: EventSource | undefined;

  onMounted(() => {
    source = new EventSource("/admin/logs");
    source.onmessage = (ev: MessageEvent<string>) => {
      lines.value.push(ev.data);
      if (lines.value.length > maxLines) {
        lines.value.splice(0, lines.value.length - maxLines);
      }
    };
  });
  onBeforeUnmount(() => {
    source?.close();
  });

  return { lines };
}
