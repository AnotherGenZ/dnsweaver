const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
if (!reducedMotion.matches && "IntersectionObserver" in window) {
  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        entry.target.classList.replace("reveal-pending", "reveal-visible");
        observer.unobserve(entry.target);
      }
    },
    { threshold: 0.08 },
  );
  for (const element of document.querySelectorAll("[data-reveal]")) {
    if (element.getBoundingClientRect().top < innerHeight) continue;
    element.classList.add("reveal-pending");
    observer.observe(element);
  }
}

const horizonControls = document.querySelector("[data-horizon-controls]");
const horizonStatus = document.querySelector("#horizon-status");
const horizonDescriptions = {
  both: "Both views: the same hostname has a different answer in each configured DNS provider.",
  internal:
    "Internal view: your internal provider answers with the address configured for your network.",
  public:
    "Public view: your public provider answers with the address configured for outside access.",
};
horizonControls.hidden = false;
for (const button of horizonControls.querySelectorAll("button")) {
  button.addEventListener("click", () => {
    const view = button.dataset.horizon;
    for (const choice of horizonControls.querySelectorAll("button"))
      choice.setAttribute("aria-pressed", String(choice === button));
    for (const resolver of document.querySelectorAll("[data-resolver]"))
      resolver.dataset.active = String(
        view === "both" || resolver.dataset.resolver === view,
      );
    horizonStatus.textContent = horizonDescriptions[view];
  });
}
