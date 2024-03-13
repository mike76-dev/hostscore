import './index.css'

type ContentProps = {
	darkMode: boolean,
	children?: React.ReactNode
}

const Content = (props: ContentProps) => {
	return (
		<div className={'content' + (props.darkMode ? ' content-dark-mode' : '')}>
			{props.children}
		</div>
	)
}

export default Content