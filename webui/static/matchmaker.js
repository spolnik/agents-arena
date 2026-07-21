(() => {
  const form = document.getElementById("matchmaker-form");
  if (!form) return;
  const red = form.elements.red_agent_id;
  const blue = form.elements.blue_agent_id;
  const submit = document.getElementById("start-match");
  const note = document.getElementById("matchup-rule");
  const pairKey = (a, b) => [a, b].sort().join(":");

  const renderPreview = (select, previewID) => {
    const preview = document.getElementById(previewID);
    const option = select.selectedOptions[0];
    if (!preview || !option) return;
    const name = option.dataset.name || option.textContent;
    preview.dataset.model = option.dataset.model || "";
    preview.querySelector('[data-role="monogram"]').textContent = name.trim().slice(0, 1).toUpperCase();
    preview.querySelector('[data-role="name"]').textContent = name;
    preview.querySelector('[data-role="owner"]').textContent = option.dataset.owner || option.dataset.author || "Owner not provided";
    preview.querySelector('[data-role="tech"]').textContent = [option.dataset.model, option.dataset.effort ? `${option.dataset.effort} effort` : ""].filter(Boolean).join(" · ");
  };

  const renderPreviews = () => {
    renderPreview(red, "red-preview");
    renderPreview(blue, "blue-preview");
  };

  fetch("/api/v1/matchups", {headers: {Accept: "application/json"}, cache: "no-store"})
    .then(response => response.ok ? response.json() : Promise.reject(new Error(`HTTP ${response.status}`)))
    .then(data => {
      const played = new Set(data.played_pairs.map(pair => pairKey(pair.agent_a_id, pair.agent_b_id)));
      const refresh = () => {
        [...blue.options].forEach(option => {
          option.disabled = option.value === red.value || played.has(pairKey(red.value, option.value));
        });
        if (blue.selectedOptions[0]?.disabled) {
          const available = [...blue.options].find(option => !option.disabled);
          if (available) blue.value = available.value;
        }
        const available = [...blue.options].some(option => !option.disabled);
        submit.disabled = !available;
        note.textContent = available
          ? "Eligible pairing · this meeting permanently closes the matchup."
          : "This agent has already faced every registered opponent.";
		renderPreviews();
      };
      red.addEventListener("change", refresh);
	  blue.addEventListener("change", renderPreviews);
      refresh();
    })
	.catch(() => {
	  note.textContent = "Pair eligibility will be verified when the match starts.";
	  renderPreviews();
	});

  renderPreviews();
})();
