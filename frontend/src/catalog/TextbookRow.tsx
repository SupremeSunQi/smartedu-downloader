import {Download} from 'lucide-react';
import type {Textbook} from '../shared/types';
import {formatBytes, normalizeStageName} from './catalogState';

interface TextbookRowProps {
  book: Textbook;
  busy: boolean;
  onDownload(): void;
}

export function TextbookRow({book, busy, onDownload}: TextbookRowProps) {
  const tags = [book.stage ? normalizeStageName(book.stage) : '', book.subject, book.edition, book.grade, book.volume].filter(Boolean);
  return (
    <article className="textbook-row">
      <div className="book-cover" aria-hidden="true">
        {book.thumbnailUrl ? <img src={book.thumbnailUrl} alt="" /> : <span>{book.subject?.slice(0, 1) || '书'}</span>}
      </div>
      <div className="book-information">
        <h3>{book.title}</h3>
        <div className="book-tags">{tags.map((tag) => <span key={tag}>{tag}</span>)}</div>
        <p>{book.provider || '智慧教育平台'}<span aria-hidden="true"> · </span>{formatBytes(book.size)}</p>
      </div>
      <button
        type="button"
        className="icon-command row-download"
        aria-label={`下载 ${book.title}`}
        title="下载"
        disabled={busy}
        onClick={onDownload}
      >
        <Download size={19} />
      </button>
    </article>
  );
}
