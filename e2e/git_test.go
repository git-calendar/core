package e2e

// func Test_AddRemote_Works(t *testing.T) {
// 	a := core.NewCore()

// 	err := a.RemoveCalendar(TestCalendarName)
// 	if err != nil {
// 		t.Errorf("failed to delete existing repo: %v", err)
// 	}

// 	err = a.CreateCalendar(TestCalendarName)
// 	if err != nil {
// 		t.Errorf("failed to init repo: %v", err)
// 	}

// 	err = a.AddRemote("github", "https://github.com/firu11/git-calendar-core.git")
// 	if err != nil {
// 		t.Errorf("failed to add remote: %v", err)
// 	}

// 	err = a.AddRemote("github", "foo")
// 	if err == nil {
// 		t.Errorf("expected an error after adding an existing remote")
// 	}

// 	err = a.AddRemote("foo", "invalid url bla bla")
// 	if err == nil {
// 		t.Errorf("expected an error after adding an invalid url")
// 	}

// 	err = a.AddRemote("bar", "https://github.com/firu11/git-calendar-core")
// 	if err == nil {
// 		t.Errorf("expected an error after adding an non-git url")
// 	}
// }
