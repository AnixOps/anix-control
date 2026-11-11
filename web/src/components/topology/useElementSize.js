import { onBeforeUnmount, onMounted, reactive } from 'vue'

// useElementSize follows the content box of the element in `target` (a ref):
// { width, height }, zero until it is mounted.
export function useElementSize(target) {
  const size = reactive({ width: 0, height: 0 })
  const observer = new ResizeObserver(([entry]) => {
    size.width = entry.contentRect.width
    size.height = entry.contentRect.height
  })
  onMounted(() => observer.observe(target.value))
  onBeforeUnmount(() => observer.disconnect())
  return size
}
