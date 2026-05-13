export interface SchemaSection {
  title: string;
  content: string;
}

export function parseSections(config: string): SchemaSection[] {
  const sections: SchemaSection[] = [];
  const lines = config.split("\n");
  let currentTitle = "";
  let currentContent: string[] = [];
  let foundFirstHeading = false;

  for (const line of lines) {
    if (line.startsWith("## ")) {
      if (foundFirstHeading && currentContent.length > 0) {
        sections.push({ title: currentTitle, content: currentContent.join("\n").trim() });
      }
      currentTitle = line.replace(/^##\s+/, "");
      currentContent = [];
      foundFirstHeading = true;
    } else if (foundFirstHeading) {
      currentContent.push(line);
    }
  }
  if (foundFirstHeading && currentContent.length > 0) {
    sections.push({ title: currentTitle, content: currentContent.join("\n").trim() });
  }
  return sections;
}

export function sectionSummary(section: SchemaSection): string {
  const lines = section.content.split("\n");
  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed && !trimmed.startsWith("#") && !trimmed.startsWith("|")) {
      return trimmed.length > 100 ? trimmed.slice(0, 97) + "..." : trimmed;
    }
  }

  const tableRows = lines.filter((line) => line.trim().startsWith("|")).length;
  if (tableRows > 2) return `${tableRows - 1} rows`;

  const listItems = lines.filter((line) => line.trim().startsWith("-")).length;
  if (listItems > 0) return `${listItems} items`;

  return "";
}
