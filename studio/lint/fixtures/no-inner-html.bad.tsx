export function bad(el: HTMLElement, s: string): void {
  el.innerHTML = s;
  el.outerHTML = s;
  el.insertAdjacentHTML("beforeend", s);
  document.write(s);
  const f = new Function("return 1");
  f();
}
