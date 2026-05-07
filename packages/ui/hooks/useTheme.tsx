"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

type Theme = "light" | "dark";
type FontSize = "small" | "medium" | "large";

const fontSizeClasses: Record<FontSize, string> = {
  small: "text-sm",
  medium: "",
  large: "text-lg",
};

const ThemeContext = createContext<{
  theme: Theme;
  fontSize: FontSize;
  setTheme: (theme: Theme) => void;
  setFontSize: (fontSize: FontSize) => void;
  toggle: () => void;
}>({
  theme: "light",
  fontSize: "medium",
  setTheme: () => {},
  setFontSize: () => {},
  toggle: () => {},
});

export function useTheme() {
  return useContext(ThemeContext);
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>("light");
  const [fontSize, setFontSizeState] = useState<FontSize>("medium");

  useEffect(() => {
    const stored = localStorage.getItem("mulwiki-theme") as Theme | null;
    if (stored) {
      setTheme(stored);
      document.documentElement.classList.toggle("dark", stored === "dark");
    } else if (window.matchMedia("(prefers-color-scheme: dark)").matches) {
      setTheme("dark");
      document.documentElement.classList.add("dark");
    }

    const storedFontSize = localStorage.getItem("mulwiki-font-size") as FontSize | null;
    if (storedFontSize && storedFontSize in fontSizeClasses) {
      setFontSizeState(storedFontSize);
      document.documentElement.classList.remove("text-sm", "text-lg");
      const className = fontSizeClasses[storedFontSize];
      if (className) document.documentElement.classList.add(className);
    }
  }, []);

  const applyTheme = (next: Theme) => {
    setTheme(next);
    localStorage.setItem("mulwiki-theme", next);
    document.documentElement.classList.toggle("dark", next === "dark");
  };

  const applyFontSize = (next: FontSize) => {
    setFontSizeState(next);
    localStorage.setItem("mulwiki-font-size", next);
    document.documentElement.classList.remove("text-sm", "text-lg");
    const className = fontSizeClasses[next];
    if (className) document.documentElement.classList.add(className);
  };

  const toggle = () => {
    applyTheme(theme === "light" ? "dark" : "light");
  };

  return (
    <ThemeContext.Provider
      value={{ theme, fontSize, setTheme: applyTheme, setFontSize: applyFontSize, toggle }}
    >
      {children}
    </ThemeContext.Provider>
  );
}
