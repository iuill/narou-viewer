export type PublicationKind = "novel" | "comic";
export type PublicationOverrideMode = "none" | "isbn" | "disabled" | "visible";
export type PublicationStatus = "unknown" | "manual" | "disabled";

export type PublicationEntry = {
  id: string;
  kind: PublicationKind;
  status: PublicationStatus;
  override: PublicationOverrideMode;
  isbn13?: string;
  title?: string;
  subtitle?: string;
  authors?: string[];
  publisher?: string;
  publishedDate?: string;
  imageUrl?: string;
  detailUrl?: string;
  source?: string;
  sourceUrl?: string;
  coverSource?: string;
  coverSourceUrl?: string;
  checkedAt?: string;
  updatedAt?: string;
  providerId?: string;
  warnings?: string[];
};

export type NovelPublicationsResponse = {
  novelId: string;
  displayCoverEntryId?: string;
  entries: PublicationEntry[];
};

export type PublicationEntryRequest = {
  kind?: PublicationKind;
  mode: PublicationOverrideMode;
  isbn13?: string;
};

export type PublicationDisplayCoverRequest = {
  entryId: string;
};
