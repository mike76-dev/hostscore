import './HostNavigation.css'
import React from 'react'

type HostNavigationProps = {
	darkMode: boolean,
	offset: number,
	limit: number,
	total: number,
	changeOffset: (offset: number) => any,
	changeLimit: (limit: number) => any
}

// Builds the list of page numbers to display: first and last page always,
// a window around the current page, gaps collapsed into ellipses.
const pageList = (current: number, totalPages: number): (number | '…')[] => {
	if (totalPages <= 7) {
		return Array.from({length: totalPages}, (_, i) => i + 1)
	}
	const pages: (number | '…')[] = [1]
	const from = Math.max(2, current - 1)
	const to = Math.min(totalPages - 1, current + 1)
	if (from > 2) pages.push('…')
	for (let p = from; p <= to; p++) pages.push(p)
	if (to < totalPages - 1) pages.push('…')
	pages.push(totalPages)
	return pages
}

export const HostNavigation = (props: HostNavigationProps) => {
	const totalPages = Math.max(1, Math.ceil(props.total / props.limit))
	const current = Math.floor(props.offset / props.limit) + 1
	const goto = (page: number) => {
		props.changeOffset((page - 1) * props.limit)
	}
	const changeRows = (event: React.ChangeEvent<HTMLSelectElement>) => {
		props.changeLimit(Number.parseInt(event.target.value))
	}
	const gotoTyped = (event: React.KeyboardEvent<HTMLInputElement>) => {
		if (event.key !== 'Enter') return
		const input = event.target as HTMLInputElement
		const page = Number.parseInt(input.value, 10)
		if (Number.isInteger(page) && page >= 1 && page <= totalPages) {
			goto(page)
		} else {
			input.value = '' + current
		}
	}
	const first = props.total === 0 ? 0 : props.offset + 1
	const last = Math.min(props.offset + props.limit, props.total)
	return (
		<div className="host-navigation-container">
			<span className="mono host-navigation-range">
				{first}–{last} of {props.total.toLocaleString('en-US')}
			</span>
			<label className="host-navigation-per-page">
				<span className="host-navigation-text">Per page</span>
				<select
					className="ctl"
					tabIndex={1}
					value={props.limit}
					onChange={changeRows}
				>
					<option value="10">10</option>
					<option value="20">20</option>
					<option value="50">50</option>
				</select>
			</label>
			<div className="host-navigation-pages">
				<button
					className="host-navigation-button"
					tabIndex={1}
					aria-label="Previous page"
					disabled={current === 1}
					onClick={() => goto(current - 1)}
				>‹</button>
				{pageList(current, totalPages).map((page, index) => (
					page === '…' ?
						<span className="host-navigation-gap" key={'gap-' + index}>…</span>
					:
						<button
							className="host-navigation-button"
							key={'page-' + page}
							tabIndex={1}
							aria-current={page === current ? 'page' : undefined}
							onClick={() => goto(page)}
						>{page}</button>
				))}
				<button
					className="host-navigation-button"
					tabIndex={1}
					aria-label="Next page"
					disabled={current >= totalPages}
					onClick={() => goto(current + 1)}
				>›</button>
				<label className="host-navigation-goto">
					<span className="host-navigation-text">Go to</span>
					<input
						className="ctl host-navigation-goto-input"
						type="number"
						min={1}
						max={totalPages}
						tabIndex={1}
						aria-label="Go to page"
						key={'goto-' + current}
						defaultValue={current}
						onKeyUp={gotoTyped}
					/>
				</label>
			</div>
		</div>
	)
}
