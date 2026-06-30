export function errorMessage(err: unknown, fallback = 'Unexpected error'): string {
  return err instanceof Error ? err.message : fallback
}

export function eventValue(event: Event): string {
  return (event.target as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement).value
}

export function eventChecked(event: Event): boolean {
  return (event.target as HTMLInputElement).checked
}

export function attr(el: Element, name: string): string {
  return el.getAttribute(name) || ''
}
