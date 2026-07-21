(() => {
  const code = document.getElementById("agent-code");
  const highlight = document.querySelector("#code-highlight code");
  const panel = document.getElementById("validation-panel");
  const title = document.getElementById("validation-title");
  const message = document.getElementById("validation-message");
  const checks = document.getElementById("validation-checks");
  const validateButton = document.getElementById("validate-code");
  const submitButton = document.getElementById("register-agent");
  const gateLabel = document.getElementById("gate-label");
  const sizeLabel = document.getElementById("code-size");
  if (!code || !highlight || !panel) return;

  const keywords = new Set(["and", "break", "continue", "def", "elif", "else", "for", "if", "in", "lambda", "load", "not", "or", "pass", "return"]);
  const constants = new Set(["True", "False", "None"]);
  const builtins = new Set(["all", "any", "bool", "dict", "enumerate", "float", "getattr", "hasattr", "hash", "int", "len", "list", "max", "min", "print", "range", "repr", "reversed", "set", "sorted", "str", "tuple", "type", "zip"]);
  const encoder = new TextEncoder();
  let validationTimer = 0;
  let validationRequest = null;
  let validatedSource = "";

  function escapeHTML(value) {
    return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
  }

  function token(className, value) {
    return `<span class="${className}">${escapeHTML(value)}</span>`;
  }

  function highlightStarlark(source) {
    let output = "";
    let index = 0;
    while (index < source.length) {
      const char = source[index];
      if (char === "#") {
        const end = source.indexOf("\n", index);
        const stop = end === -1 ? source.length : end;
        output += token("tok-comment", source.slice(index, stop));
        index = stop;
        continue;
      }
      if (char === '"' || char === "'") {
        const triple = source.slice(index, index + 3) === char.repeat(3);
        const delimiter = triple ? char.repeat(3) : char;
        let end = index + delimiter.length;
        while (end < source.length) {
          if (source.slice(end, end + delimiter.length) === delimiter && source[end - 1] !== "\\") {
            end += delimiter.length;
            break;
          }
          end++;
        }
        output += token("tok-string", source.slice(index, end));
        index = end;
        continue;
      }
      const identifier = source.slice(index).match(/^[A-Za-z_][A-Za-z0-9_]*/)?.[0];
      if (identifier) {
        const className = keywords.has(identifier) ? "tok-keyword" : constants.has(identifier) ? "tok-constant" : builtins.has(identifier) ? "tok-builtin" : "tok-identifier";
        output += token(className, identifier);
        index += identifier.length;
        continue;
      }
      const number = source.slice(index).match(/^(?:0[xX][0-9a-fA-F]+|\d+(?:\.\d+)?)/)?.[0];
      if (number) {
        output += token("tok-number", number);
        index += number.length;
        continue;
      }
      if ("+-*/%=<>!&|^~:,.()[]{}".includes(char)) {
        output += token("tok-operator", char);
      } else {
        output += escapeHTML(char);
      }
      index++;
    }
    return output + (source.endsWith("\n") ? " " : "\n");
  }

  function renderCode() {
    highlight.innerHTML = highlightStarlark(code.value);
    highlight.parentElement.scrollTop = code.scrollTop;
    highlight.parentElement.scrollLeft = code.scrollLeft;
    const bytes = encoder.encode(code.value).length;
    sizeLabel.textContent = `${bytes.toLocaleString()} B / 64 KiB`;
    sizeLabel.dataset.over = bytes > 65536 ? "true" : "false";
  }

  function setValidation(state, heading, detail, items = []) {
    panel.dataset.state = state;
    title.textContent = heading;
    message.textContent = detail;
    checks.innerHTML = items.map((item) => `<li>${escapeHTML(item)}</li>`).join("");
    const valid = state === "valid" && validatedSource === code.value;
    submitButton.disabled = !valid;
    gateLabel.textContent = valid ? "Code verified · ready to register" : state === "invalid" ? "Fix validation errors first" : "Waiting for valid code";
  }

  async function validateCode() {
    window.clearTimeout(validationTimer);
    validationRequest?.abort();
    const source = code.value;
    validatedSource = "";
    if (!source.trim()) {
      setValidation("invalid", "Code is required", "Define choose_move(state) before registering.");
      return;
    }
    setValidation("checking", "Checking script…", "Parsing and running a sample decision in the arena runtime.");
    validationRequest = new AbortController();
    try {
      const response = await fetch("/api/v1/agents/validate", {
        method: "POST",
        headers: {"Content-Type": "application/json", "Accept": "application/json"},
        body: JSON.stringify({code: source}),
        signal: validationRequest.signal,
      });
      const result = await response.json();
      if (!response.ok || !result.valid) {
        setValidation("invalid", "Script rejected", result.error || "The runtime rejected this script.");
        return;
      }
      validatedSource = source;
      setValidation("valid", "Script is valid", result.message, result.checks || []);
    } catch (error) {
      if (error.name === "AbortError") return;
      setValidation("invalid", "Validation unavailable", "Could not reach the arena runtime. Try again.");
    }
  }

  function scheduleValidation() {
    validatedSource = "";
    submitButton.disabled = true;
    setValidation("checking", "Waiting for pause…", "Validation starts automatically after you stop typing.");
    window.clearTimeout(validationTimer);
    validationTimer = window.setTimeout(validateCode, 450);
  }

  code.addEventListener("input", () => { renderCode(); scheduleValidation(); });
  code.addEventListener("scroll", () => {
    highlight.parentElement.scrollTop = code.scrollTop;
    highlight.parentElement.scrollLeft = code.scrollLeft;
  });
  code.addEventListener("keydown", (event) => {
    if (event.key !== "Tab") return;
    event.preventDefault();
    const start = code.selectionStart;
    const end = code.selectionEnd;
    code.setRangeText("    ", start, end, "end");
    code.dispatchEvent(new Event("input", {bubbles: true}));
  });
  validateButton.addEventListener("click", validateCode);

  [["input[name='name']", "name-count"], ["input[name='author']", "author-count"], ["#agent-description", "description-count"], ["input[name='owner_name']", "owner-name-count"], ["input[name='model']", "model-count"]].forEach(([selector, outputID]) => {
    const input = document.querySelector(selector);
    const output = document.getElementById(outputID);
    input?.addEventListener("input", () => { output.textContent = input.value.length; });
  });

  document.querySelector(".workbench")?.addEventListener("htmx:beforeRequest", (event) => {
    if (validatedSource !== code.value) {
      event.preventDefault();
      validateCode();
      return;
    }
    submitButton.textContent = "Registering…";
    submitButton.disabled = true;
  });
  document.querySelector(".workbench")?.addEventListener("htmx:afterRequest", () => {
    submitButton.textContent = "Register agent & join roster";
    submitButton.disabled = validatedSource !== code.value;
  });

  renderCode();
  validateCode();
})();
