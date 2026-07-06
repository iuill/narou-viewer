export type PaginationResult<TItem> = {
  items: TItem[];
  currentPage: number;
  totalPages: number;
  totalItems: number;
  startItemNumber: number;
  endItemNumber: number;
};

export function paginateItems<TItem>(items: TItem[], requestedPage: number, pageSize: number): PaginationResult<TItem> {
  const normalizedPageSize = Number.isInteger(pageSize) && pageSize > 0 ? pageSize : Math.max(items.length, 1);
  const totalItems = items.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / normalizedPageSize));
  const currentPage =
    Number.isInteger(requestedPage) && requestedPage > 0 ? Math.min(requestedPage, totalPages) : 1;
  const startIndex = totalItems === 0 ? 0 : (currentPage - 1) * normalizedPageSize;
  const endIndex = totalItems === 0 ? 0 : Math.min(startIndex + normalizedPageSize, totalItems);

  return {
    items: items.slice(startIndex, endIndex),
    currentPage,
    totalPages,
    totalItems,
    startItemNumber: totalItems === 0 ? 0 : startIndex + 1,
    endItemNumber: endIndex
  };
}
