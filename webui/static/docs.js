(() => {
  document.querySelectorAll("[data-copy-target]").forEach((button) => {
    button.addEventListener("click", async () => {
      const target = document.getElementById(button.dataset.copyTarget);
      if (!target) return;
      const original = button.textContent;
      const text = target.innerText.trim();
      try {
        await navigator.clipboard.writeText(text);
        button.textContent = "Copied";
      } catch {
        const fallback = document.createElement("textarea");
        fallback.value = text;
        fallback.setAttribute("readonly", "");
        fallback.style.position = "fixed";
        fallback.style.opacity = "0";
        document.body.appendChild(fallback);
        fallback.select();
        const copied = document.execCommand("copy");
        fallback.remove();
        button.textContent = copied ? "Copied" : "Select & copy";
      }
      window.setTimeout(() => { button.textContent = original; }, 3000);
    });
  });
})();
