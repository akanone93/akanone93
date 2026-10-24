declare function t(key: string): string;
export const a = <button type="button">{t("save")}</button>;
export const b = <input placeholder={t("search")} />;
export const c = <img src="x.png" alt={t("logo")} />;
export const d = <button title={t("close")} aria-label={t("close")} type="button" />;
export const e = <span> </span>;
