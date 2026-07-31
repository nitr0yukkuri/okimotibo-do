export type Mood = "busy" | "available" | "neutral";

export const moodOptions: Array<{ id: Mood; label: string }> = [
  { id: "available", label: "したい" },
  { id: "neutral", label: "いいよ" },
  { id: "busy", label: "したくない" },
];

export const moodLabel: Record<Mood, string> = {
  available: "したい",
  neutral: "いいよ",
  busy: "したくない",
};

export const moodColor: Record<Mood, string> = {
  available: "#94f89b",
  neutral: "#ffe98d",
  busy: "#ff9390",
};
