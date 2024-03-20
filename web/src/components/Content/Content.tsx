import './Content.css'

type ContentProps = {
	darkMode: boolean,
	children?: React.ReactNode
}

export const Content = (props: ContentProps) => {
	return (
		<div className={'content' + (props.darkMode ? ' content-dark-mode' : '')}>
			{props.children}
		</div>
	)
}
