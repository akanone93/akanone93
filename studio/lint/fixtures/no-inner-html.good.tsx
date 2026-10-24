export function good(el: HTMLElement, s: string): void {
  el.textContent = s;
  el.append(document.createTextNode(s));
}
