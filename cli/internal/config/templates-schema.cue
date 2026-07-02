// Template schema definitions
// All templates must produce concepts that conform to #Concept

// Template metadata
#TemplateMeta: {
	name:        string & !=""
	description: string
	tags:        [...(string & !="")]
}

// Standard templates
#MeetingTemplate: #TemplateMeta & {
	name:        "meeting"
	description: "Meeting notes with attendees and action items"
	tags:        ["meeting"]
}

#BookReviewTemplate: #TemplateMeta & {
	name:        "book-review"
	description: "Book review with rating and key takeaways"
	tags:        ["book", "review"]
}

#SnippetTemplate: #TemplateMeta & {
	name:        "snippet"
	description: "Code snippet with context and explanation"
	tags:        ["code", "snippet"]
}

// Custom templates
#ProjectIdeaTemplate: #TemplateMeta & {
	name:        "project-idea"
	description: "Project idea with goals and next steps"
	tags:        ["idea", "project"]
}

#UserStoryTemplate: #TemplateMeta & {
	name:        "user-story"
	description: "User story in standard format with acceptance criteria"
	tags:        ["user-story", "requirements"]
}

#FeatureTemplate: #TemplateMeta & {
	name:        "feature"
	description: "Feature specification with requirements and design notes"
	tags:        ["feature", "spec"]
}
