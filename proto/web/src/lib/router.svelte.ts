// A minimal hash router: the app is served as static files by the Go server,
// so hash routes need no server-side fallback.

function current() {
  return location.hash.replace(/^#/, "") || "/";
}

export const route = $state({ path: current() });

window.addEventListener("hashchange", () => {
  route.path = current();
  window.scrollTo(0, 0);
});

export function navigate(path: string) {
  location.hash = path;
}
