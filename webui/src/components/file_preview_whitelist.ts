export type FilePreviewMode = "txt" | "image" | "video" | "none";

const TXT_EXTENSIONS = new Set([
  "txt", "md", "json", "yaml", "yml", "toml", "xml", "html", "css",
  "js", "mjs", "cjs", "ts", "tsx", "jsx", "vue", "go", "py", "sh",
  "bash", "zsh", "sql", "ini", "conf", "log", "gitignore", "env"
]);

const IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "gif", "webp", "bmp", "svg"]);
const VIDEO_EXTENSIONS = new Set(["mp4", "webm", "ogg", "mov", "m4v"]);

function getExtension(path: string): string {
  const name = String(path ?? "").trim().split("/").pop() ?? "";
  const idx = name.lastIndexOf(".");
  if (idx <= 0 || idx === name.length - 1) {
    return "";
  }
  return name.slice(idx + 1).toLowerCase();
}

export function getFilePreviewMode(path: string): FilePreviewMode {
  const ext = getExtension(path);
  if (TXT_EXTENSIONS.has(ext)) {
    return "txt";
  }
  if (IMAGE_EXTENSIONS.has(ext)) {
    return "image";
  }
  if (VIDEO_EXTENSIONS.has(ext)) {
    return "video";
  }
  return "none";
}
