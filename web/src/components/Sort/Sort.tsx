import { ReactNode } from 'react'
import './Sort.css'

type SortProps = {
	darkMode: boolean,
	order: 'asc' | 'desc' | 'none'
	setOrder: (order: 'asc' | 'desc') => any
	children?: ReactNode
}

export const Sort = (props: SortProps) => {
	const changeOrder = () => {
		props.setOrder(props.order === 'asc' ? 'desc' : 'asc')
	}
	return (
		<div
			className={'sort-container' + (props.darkMode ? ' sort-dark' : '')}
			tabIndex={1}
			onClick={changeOrder}
			onKeyUp={(event: React.KeyboardEvent<HTMLDivElement>) => {
				if (event.key === 'Enter' || event.key === ' ') {
					changeOrder()
				}
			}}
		>
			{props.children}
			<span className="sort-icon-container">
				<svg viewBox="0 0 64 64">
					<path className={
						props.order === 'asc' ? 'sort-path-active' : 'sort-path'
					} d="M32 0 L46 26 L15 26 Z"/>
					<path className={
						props.order === 'desc' ? 'sort-path-active' : 'sort-path'
					} d="M32 63 L46 37 L15 37 Z"/>
				</svg>
			</span>
		</div>
	)
}