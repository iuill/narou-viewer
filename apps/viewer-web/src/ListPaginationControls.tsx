type Props = {
  label: string;
  ariaLabel?: string;
  currentPage: number;
  totalPages: number;
  totalItems: number;
  startItemNumber: number;
  endItemNumber: number;
  onPageChange: (page: number) => void;
};

export function ListPaginationControls({
  label,
  ariaLabel,
  currentPage,
  totalPages,
  totalItems,
  startItemNumber,
  endItemNumber,
  onPageChange
}: Props) {
  if (totalItems === 0) {
    return null;
  }

  return (
    <nav aria-label={ariaLabel ?? `${label}ページ切り替え`} className="list-pagination">
      <p className="list-pagination-summary">
        {startItemNumber}-{endItemNumber} / {totalItems} 件
      </p>
      <div className="list-pagination-controls">
        <button disabled={currentPage <= 1} onClick={() => onPageChange(currentPage - 1)} type="button">
          前へ
        </button>
        <span>
          {currentPage} / {totalPages} ページ
        </span>
        <button disabled={currentPage >= totalPages} onClick={() => onPageChange(currentPage + 1)} type="button">
          次へ
        </button>
      </div>
    </nav>
  );
}
